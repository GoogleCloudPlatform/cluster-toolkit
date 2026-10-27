// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package subblockmap lists the sub-blocks of a reservation and selects the
// key -> sub-block path map consumed by the gke-node-pool
// reservation_affinity.specific_reservations[0].reservation_subblocks input.
//
// Each sub-block gets the key "BB-SSSS" (block number, sub-block number), e.g.
// my-res-block-0002-subblock-0001 -> "02-0001". Keys depend only on the
// sub-block's name, so the same sub-block always gets the same key and
// gke-node-pool keeps its node pool when other entries are added or removed.
package subblockmap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
)

const (
	maxBlock    = 99
	maxSubBlock = 9999
	// RequiredPermissions are needed on the project that owns the reservation.
	RequiredPermissions = "compute.reservationBlocks.list and compute.reservationSubBlocks.list"
)

var (
	blockNumRe    = regexp.MustCompile(`-block-(\d+)$`)
	subBlockNumRe = regexp.MustCompile(`-subblock-(\d+)$`)
)

// Reservation identifies a reservation.
type Reservation struct {
	Project string // project that owns the reservation
	Zone    string
	Name    string // bare reservation name
}

// SubBlock is one reservation sub-block.
type SubBlock struct {
	Block string
	Name  string
	Path  string // RESERVATION/reservationBlocks/BLOCK/reservationSubBlocks/SUBBLOCK
	Count int64
	InUse int64
	// Healthy reports whether the sub-block can be used: status READY and health
	// HEALTHY or DEGRADED. Degraded is set for DEGRADED.
	Healthy  bool
	Degraded bool
}

// Free reports whether no host of the sub-block is in use.
func (s SubBlock) Free() bool { return s.InUse == 0 }

// Lister lists the blocks and sub-blocks of a reservation.
type Lister interface {
	Blocks(ctx context.Context, project, zone, reservation string) ([]string, error)
	SubBlocks(ctx context.Context, project, zone, reservation, block string) ([]*compute.ReservationSubBlock, error)
}

// ComputeLister lists reservation blocks and sub-blocks with the Compute Engine API,
// using Application Default Credentials (the same credentials gcluster and Terraform use).
type ComputeLister struct {
	Service *compute.Service
}

// NewComputeLister returns a Lister backed by the Compute Engine API.
func NewComputeLister(ctx context.Context) (*ComputeLister, error) {
	s, err := compute.NewService(ctx)
	if err != nil {
		return nil, err
	}
	return &ComputeLister{Service: s}, nil
}

// Blocks returns the names of the reservation's blocks.
func (l *ComputeLister) Blocks(ctx context.Context, project, zone, reservation string) ([]string, error) {
	var names []string
	err := l.Service.ReservationBlocks.List(project, zone, reservation).Pages(ctx, func(r *compute.ReservationBlocksListResponse) error {
		for _, b := range r.Items {
			names = append(names, b.Name)
		}
		return nil
	})
	return names, err
}

// SubBlocks returns the sub-blocks of one reservation block.
func (l *ComputeLister) SubBlocks(ctx context.Context, project, zone, reservation, block string) ([]*compute.ReservationSubBlock, error) {
	var items []*compute.ReservationSubBlock
	parent := fmt.Sprintf("reservations/%s/reservationBlocks/%s", reservation, block)
	err := l.Service.ReservationSubBlocks.List(project, zone, parent).Pages(ctx, func(r *compute.ReservationSubBlocksListResponse) error {
		items = append(items, r.Items...)
		return nil
	})
	return items, err
}

// IsPermissionError reports whether err is a 401 or 403 from the Compute Engine API.
func IsPermissionError(err error) bool {
	var gerr *googleapi.Error
	return errors.As(err, &gerr) && (gerr.Code == http.StatusForbidden || gerr.Code == http.StatusUnauthorized)
}

// ListSubBlocks lists all sub-blocks of the reservation, sorted by block and name.
func ListSubBlocks(ctx context.Context, l Lister, r Reservation) ([]SubBlock, error) {
	wrap := func(err error) error {
		if IsPermissionError(err) {
			return fmt.Errorf("%w\nhint: listing sub-blocks requires %s (e.g. roles/compute.viewer) on project %q, which owns the reservation",
				err, RequiredPermissions, r.Project)
		}
		return fmt.Errorf("listing sub-blocks of reservation %q (project %q, zone %q): %w", r.Name, r.Project, r.Zone, err)
	}

	blocks, err := l.Blocks(ctx, r.Project, r.Zone, r.Name)
	if err != nil {
		return nil, wrap(err)
	}
	if len(blocks) == 0 {
		return nil, fmt.Errorf("reservation %q (project %q, zone %q) has no blocks; is it a dense reservation?", r.Name, r.Project, r.Zone)
	}
	sort.Strings(blocks)

	var out []SubBlock
	for _, b := range blocks {
		items, err := l.SubBlocks(ctx, r.Project, r.Zone, r.Name, b)
		if err != nil {
			return nil, wrap(err)
		}
		for _, sb := range items {
			health := "HEALTHY"
			if sb.HealthInfo != nil && sb.HealthInfo.HealthStatus != "" {
				health = sb.HealthInfo.HealthStatus
			}
			status := sb.Status
			if status == "" {
				status = "READY"
			}
			out = append(out, SubBlock{
				Block:    b,
				Name:     sb.Name,
				Path:     fmt.Sprintf("%s/reservationBlocks/%s/reservationSubBlocks/%s", r.Name, b, sb.Name),
				Count:    sb.Count,
				InUse:    sb.InUseCount,
				Healthy:  status == "READY" && (health == "HEALTHY" || health == "DEGRADED"),
				Degraded: health == "DEGRADED",
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Block != out[j].Block {
			return out[i].Block < out[j].Block
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Key returns the map key "BB-SSSS" for a block and sub-block name.
func Key(block, subBlock string) (string, error) {
	b := blockNumRe.FindStringSubmatch(block)
	sb := subBlockNumRe.FindStringSubmatch(subBlock)
	if b == nil || sb == nil {
		return "", fmt.Errorf("cannot build a key for sub-block %q in block %q: names must end in '-block-<number>' and '-subblock-<number>'", subBlock, block)
	}
	bn, _ := strconv.Atoi(b[1])
	sn, _ := strconv.Atoi(sb[1])
	if bn > maxBlock || sn > maxSubBlock {
		return "", fmt.Errorf("sub-block %q: block number %d or sub-block number %d is too large for the key format BB-SSSS (max block %d, max sub-block %d)",
			subBlock, bn, sn, maxBlock, maxSubBlock)
	}
	return fmt.Sprintf("%02d-%04d", bn, sn), nil
}

// Selection is the result of Select.
type Selection struct {
	Map      map[string]string // key -> sub-block path, exactly n entries
	Kept     []string          // keys taken over from the previous map (sorted)
	Added    []string          // keys of newly selected sub-blocks (sorted)
	Removed  []string          // keys dropped from the previous map (sorted)
	Warnings []string
}

// Select returns a map of exactly n sub-blocks of the reservation.
//
// Entries of previous (the saved map) are kept, so existing node pools don't move.
// If previous has more than n entries, the highest keys are dropped. If it has
// fewer, list is called and free sub-blocks are added, skipping paths in exclude
// (sub-blocks used by other node pool modules): HEALTHY ones with the lowest keys
// first, then DEGRADED ones (with a warning).
// A previous entry is never dropped because its sub-block is busy or unhealthy:
// its node pool is the one using it.
func Select(previous map[string]string, n int, reservation string, exclude map[string]bool, list func() ([]SubBlock, error)) (Selection, error) {
	if n < 1 {
		return Selection{}, fmt.Errorf("num_node_pools must be at least 1, got %d", n)
	}
	sel := keepPrevious(previous, n, reservation)
	if len(sel.Map) == n {
		return sel, nil
	}

	subs, err := list()
	if err != nil {
		return Selection{}, err
	}
	sel.Warnings = append(sel.Warnings, keptWarnings(sel, subs, reservation)...)

	used := map[string]bool{}
	for p := range exclude {
		used[p] = true
	}
	for _, p := range sel.Map {
		used[p] = true
	}
	added, degraded, err := addFree(sel.Map, subs, used, n-len(sel.Map))
	if err != nil {
		return Selection{}, err
	}
	if len(sel.Map) < n {
		return Selection{}, fmt.Errorf("not enough free sub-blocks in reservation %q: %d more needed (num_node_pools: %d, entries in reservation_subblocks: %d), %d available.\n"+
			"A sub-block is available only if it is READY, HEALTHY or DEGRADED, and none of its hosts are in use, so sub-blocks used by existing node pools don't count.\n"+
			"Lower num_node_pools, or run again when a sub-block is free",
			reservation, n-len(sel.Kept), n, len(sel.Kept), len(added))
	}
	for _, k := range degraded {
		sel.Warnings = append(sel.Warnings, fmt.Sprintf("proposed sub-block (%q: %s) is DEGRADED: some of its hosts may be unhealthy, so fewer nodes may be created than requested", k, sel.Map[k]))
	}
	sel.Added = added
	return sel, nil
}

// keepPrevious takes over the entries of previous that belong to the reservation,
// dropping the highest keys if there are more than n.
func keepPrevious(previous map[string]string, n int, reservation string) Selection {
	sel := Selection{Map: map[string]string{}}
	prefix := reservation + "/reservationBlocks/"
	var dropped []string
	for k, p := range previous {
		if strings.HasPrefix(p, prefix) {
			sel.Map[k] = p
		} else {
			dropped = append(dropped, k)
		}
	}
	if len(dropped) > 0 {
		sort.Strings(dropped)
		sel.Removed = append(sel.Removed, dropped...)
		sel.Warnings = append(sel.Warnings, fmt.Sprintf("saved sub-blocks %v are not in reservation %q and are removed from the map; their node pools will be replaced", dropped, reservation))
	}
	keys := sortedKeys(sel.Map)
	if len(keys) > n {
		for _, k := range keys[n:] {
			delete(sel.Map, k)
			sel.Removed = append(sel.Removed, k)
		}
		keys = keys[:n]
	}
	sel.Kept = keys
	sort.Strings(sel.Removed)
	return sel
}

// keptWarnings reports kept entries whose sub-block is missing or unhealthy.
func keptWarnings(sel Selection, subs []SubBlock, reservation string) []string {
	byPath := map[string]SubBlock{}
	for _, s := range subs {
		byPath[s.Path] = s
	}
	var warnings []string
	for _, k := range sel.Kept {
		s, ok := byPath[sel.Map[k]]
		switch {
		case !ok:
			warnings = append(warnings, fmt.Sprintf("saved sub-block (%q: %s) is no longer in reservation %q; kept in the map", k, sel.Map[k], reservation))
		case !s.Healthy:
			warnings = append(warnings, fmt.Sprintf("saved sub-block (%q: %s) is not healthy; kept in the map", k, sel.Map[k]))
		case s.Degraded:
			warnings = append(warnings, fmt.Sprintf("saved sub-block (%q: %s) is DEGRADED; kept in the map", k, sel.Map[k]))
		}
	}
	return warnings
}

// addFree adds up to need free, usable sub-blocks not in used to m: HEALTHY ones
// in list order (lowest keys first), then DEGRADED ones. It returns the keys of
// all added sub-blocks and of the DEGRADED ones among them, both sorted.
func addFree(m map[string]string, subs []SubBlock, used map[string]bool, need int) ([]string, []string, error) {
	var added, degraded []string
	for _, wantDegraded := range []bool{false, true} {
		for _, s := range subs {
			if len(added) == need {
				break
			}
			if used[s.Path] || !s.Healthy || !s.Free() || s.Degraded != wantDegraded {
				continue
			}
			key, err := Key(s.Block, s.Name)
			if err != nil {
				return nil, nil, err
			}
			if cur, ok := m[key]; ok {
				return nil, nil, fmt.Errorf("sub-blocks %s and %s both get key %q", cur, s.Path, key)
			}
			m[key] = s.Path
			added = append(added, key)
			if s.Degraded {
				degraded = append(degraded, key)
			}
		}
	}
	sort.Strings(added)
	sort.Strings(degraded)
	return added, degraded, nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// RenderVar returns the YAML lines for `name: <map>` with the given indent and
// sorted, quoted keys.
func RenderVar(name string, m map[string]string, indent string) []string {
	if len(m) == 0 {
		return []string{fmt.Sprintf("%s%s: {}\n", indent, name)}
	}
	lines := []string{fmt.Sprintf("%s%s:\n", indent, name)}
	for _, k := range sortedKeys(m) {
		lines = append(lines, fmt.Sprintf("%s  %q: %s\n", indent, k, m[k]))
	}
	return lines
}

var varsLineRe = regexp.MustCompile(`^vars:\s*(#.*)?$`)

// UpdateVar replaces the variable `name` under the top-level `vars:` block of a
// YAML document with m, or inserts it at the end of that block. Only those lines
// change; the rest of the text, including comments, is kept.
func UpdateVar(text, name string, m map[string]string) (string, error) {
	lines := splitLines(text)
	varsIdx, end, indent, err := varsBlock(lines)
	if err != nil {
		return "", err
	}
	rendered := RenderVar(name, m, indent)
	start, stop := varLines(lines, varsIdx+1, end, indent, name)
	if start < 0 {
		// Not set yet: insert at the end of the vars block, before trailing blank lines.
		start = end
		for start > varsIdx+1 && isBlank(lines[start-1]) {
			start--
		}
		stop = start
	}
	out := append(append(append([]string{}, lines[:start]...), rendered...), lines[stop:]...)
	return strings.Join(out, ""), nil
}

// splitLines splits text into lines that all end in "\n".
func splitLines(text string) []string {
	lines := strings.SplitAfter(text, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	if n := len(lines); n > 0 && !strings.HasSuffix(lines[n-1], "\n") {
		lines[n-1] += "\n"
	}
	return lines
}

func isBlank(l string) bool { return strings.TrimSpace(l) == "" }

func indentOf(l string) int { return len(l) - len(strings.TrimLeft(l, " \t")) }

// varsBlock returns the index of the top-level "vars:" line, the end of its block
// (following lines that are blank, comments, or indented) and the indent of its
// entries.
func varsBlock(lines []string) (int, int, string, error) {
	varsIdx := -1
	for i, l := range lines {
		if varsLineRe.MatchString(strings.TrimRight(l, "\n")) {
			varsIdx = i
			break
		}
	}
	if varsIdx < 0 {
		return 0, 0, "", errors.New("file has no top-level 'vars:' block")
	}
	end := varsIdx + 1
	for end < len(lines) && (isBlank(lines[end]) || strings.ContainsAny(lines[end][:1], " \t#")) {
		end++
	}
	indent := "  "
	for _, l := range lines[varsIdx+1 : end] {
		if !isBlank(l) && !strings.HasPrefix(strings.TrimSpace(l), "#") {
			indent = l[:indentOf(l)]
			break
		}
	}
	return varsIdx, end, indent, nil
}

// varLines returns the line range [start, stop) of variable name within
// lines[from:end], or -1, -1 if it isn't set. Trailing blank lines are excluded.
func varLines(lines []string, from, end int, indent, name string) (int, int) {
	keyRe := regexp.MustCompile("^" + regexp.QuoteMeta(indent) + regexp.QuoteMeta(name) + `\s*:`)
	for start := from; start < end; start++ {
		if !keyRe.MatchString(lines[start]) {
			continue
		}
		stop := start + 1
		for stop < end && (isBlank(lines[stop]) || indentOf(lines[stop]) > len(indent)) {
			stop++
		}
		for stop > start+1 && isBlank(lines[stop-1]) {
			stop--
		}
		return start, stop
	}
	return -1, -1
}

// Diff returns a unified diff of the single changed region between before and
// after (UpdateVar only ever changes one contiguous block of lines).
func Diff(before, after, fromFile, toFile string) string {
	a := strings.SplitAfter(before, "\n")
	b := strings.SplitAfter(after, "\n")
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	if pre == len(a) && pre == len(b) {
		return ""
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n+++ %s\n", fromFile, toFile)
	for _, l := range a[pre : len(a)-suf] {
		sb.WriteString("-" + strings.TrimSuffix(l, "\n") + "\n")
	}
	for _, l := range b[pre : len(b)-suf] {
		sb.WriteString("+" + strings.TrimSuffix(l, "\n") + "\n")
	}
	return sb.String()
}
