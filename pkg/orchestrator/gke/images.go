// Copyright 2026 "Google LLC"
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gke

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/k8s"
)

const (
	defaultPathwaysProxyImage  = "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/proxy_server:latest"
	defaultPathwaysServerImage = "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server:latest"

	// publicPathwaysImageRepo publishes the public Pathways server, proxy and colocated Python sidecar images. Its
	// sidecar only works with the server and proxy images of the same repository, not with gcluster's defaults above:
	// the public Pathways colocated Python guide tells Cluster Toolkit users to pass the server and proxy images with
	// the sidecar's tag, and every sidecar release tag (publicPathwaysReleaseTag) is also published for the server and
	// proxy images, which are the same build for every JAX version of a release.
	publicPathwaysImageRepo    = "us-docker.pkg.dev/cloud-tpu-v2-images/cloud-pathways"
	publicPathwaysSidecarImage = publicPathwaysImageRepo + "/colocated_python_sidecar"
	publicPathwaysServerImage  = publicPathwaysImageRepo + "/server"
	publicPathwaysProxyImage   = publicPathwaysImageRepo + "/proxy_server"
	pathwaysSidecarImagesDoc   = `"Colocated Python sidecar images" in docs/gcluster_job_guide.md`
)

// publicPathwaysReleaseTag matches the release tags of the public Pathways images: jax-<version> (rolling),
// <yyyymmdd>-jax_<version> (one day's build) and latest (the newest JAX version).
var publicPathwaysReleaseTag = regexp.MustCompile(`^(latest|jax-[0-9]+(?:\.[0-9]+)*|[0-9]{8}-jax_[0-9]+(?:\.[0-9]+)*)$`)

// resolvePathwaysImages fills in the Pathways server, proxy and worker images the user did not set. With the public
// colocated Python sidecar every missing server or proxy image comes from publicPathwaysImageRepo at the sidecar's
// release; explicitly set images are never changed. A digest-pinned public sidecar without a release tag returns an
// error unless the required server/proxy images are explicitly provided. It returns the message to log about the
// images it chose, if any, whether that message is a warning, and any resolution error.
func resolvePathwaysImages(p orchestrator.PathwaysJobDefinition) (orchestrator.PathwaysJobDefinition, string, bool, error) {
	var msg string
	var warn bool
	var err error
	if tag, exact, pinnedByDigest, ok := publicSidecarRelease(p.ColocatedPythonSidecarImage); ok {
		msg, warn, err = fillFromPublicSidecarRelease(&p, tag, exact, pinnedByDigest)
		if err != nil {
			return p, "", false, err
		}
	} else if p.ColocatedPythonSidecarImage != "" {
		msg = customSidecarNotice(p)
		warn = msg != ""
	}
	if p.ProxyServerImage == "" {
		p.ProxyServerImage = defaultPathwaysProxyImage
	}
	if p.ServerImage == "" {
		p.ServerImage = defaultPathwaysServerImage
	}
	if p.WorkerImage == "" {
		p.WorkerImage = p.ServerImage
	}
	return p, msg, warn, nil
}

// publicSidecarRelease reports whether image is the public sidecar and returns its release tag. exact is false when
// the reference names no release (a bare digest or another tag), in which case the tag is latest. pinnedByDigest is
// true when the reference includes a digest (@sha256:...), which container runtimes pull instead of the tag.
func publicSidecarRelease(image string) (tag string, exact, pinnedByDigest, ok bool) {
	rest, found := strings.CutPrefix(image, publicPathwaysSidecarImage)
	if !found || (rest != "" && rest[0] != ':' && rest[0] != '@') {
		return "", false, false, false
	}
	ref, _, hasDigest := strings.Cut(rest, "@")
	switch {
	case ref == "" && !hasDigest:
		return "latest", true, false, true // A bare reference pulls latest.
	case ref != "" && publicPathwaysReleaseTag.MatchString(ref[1:]):
		return ref[1:], true, hasDigest, true
	default:
		return "latest", false, hasDigest, true
	}
}

// proxyImageFor derives the proxy image that matches an SPS server image, as pathwaysutils does.
func proxyImageFor(serverImage string) string {
	// Pathways server and proxy images never share a digest, so only a tag can carry over.
	if strings.Contains(serverImage, "@") {
		return ""
	}
	if prefix, ok := strings.CutSuffix(serverImage, "/server"); ok && prefix != "" {
		return prefix + "/proxy_server:latest"
	}
	i := strings.LastIndex(serverImage, "/server:")
	if i <= 0 {
		return ""
	}
	tag := serverImage[i+len("/server:"):]
	if tag == "" || strings.Contains(tag, "/") {
		return ""
	}
	return serverImage[:i] + "/proxy_server:" + tag
}

// pathwaysImageNouns names the images the rendered JobSet runs besides the sidecar, and the flags that set them.
// A deploy-SPS head runs only pathways-rm, because each SPS client brings its own proxy.
func pathwaysImageNouns(deploySPS bool) (images, flags string) {
	if deploySPS {
		return "server image", "--pathways-server-image"
	}
	return "server or proxy image", "--pathways-server-image and --pathways-proxy-server-image"
}

// fillFromPublicSidecarRelease sets every missing server or proxy image to the public image of the given release and
// returns the message describing what it chose, or an error when a digest-pinned sidecar has no release tag to pair.
func fillFromPublicSidecarRelease(p *orchestrator.PathwaysJobDefinition, tag string, exact, pinnedByDigest bool) (string, bool, error) {
	if !exact && pinnedByDigest {
		if p.ServerImage == "" || (!p.DeploySPS && p.ProxyServerImage == "") {
			images, flags := pathwaysImageNouns(p.DeploySPS)
			return "", false, fmt.Errorf("public colocated Python sidecar %q is pinned by digest without a release tag, so gcluster cannot determine the matching release for the unset Pathways %s; pass %s (or use a release tag like jax-<version> or <yyyymmdd>-jax_<version>)",
				p.ColocatedPythonSidecarImage, images, flags)
		}
		return "", false, nil
	}
	var chosen, kept []string
	if p.ServerImage == "" {
		p.ServerImage = publicPathwaysServerImage + ":" + tag
		chosen = append(chosen, "server image "+p.ServerImage)
	} else {
		kept = append(kept, "--pathways-server-image")
	}
	if !p.DeploySPS {
		if p.ProxyServerImage == "" {
			p.ProxyServerImage = publicPathwaysProxyImage + ":" + tag
			chosen = append(chosen, "proxy image "+p.ProxyServerImage)
		} else {
			kept = append(kept, "--pathways-proxy-server-image")
		}
	}
	if p.WorkerImage != "" {
		kept = append(kept, "--pathways-worker-image")
	}
	if len(chosen) == 0 {
		return "", false, nil
	}
	msg, warn := formatPublicSidecarNotice(p.ColocatedPythonSidecarImage, p.DeploySPS, chosen, kept, tag, exact, pinnedByDigest)
	return msg, warn, nil
}

func formatPublicSidecarNotice(sidecar string, deploySPS bool, chosen, kept []string, tag string, exact, pinnedByDigest bool) (string, bool) {
	images, flags := pathwaysImageNouns(deploySPS)
	msg := fmt.Sprintf("Using the Pathways %s for the public colocated Python sidecar %s: with the public sidecar, gcluster sets every Pathways %s you do not pass to the %s image of the sidecar's release.",
		strings.Join(chosen, " and "), sidecar, images, publicPathwaysImageRepo)
	var pinNote string
	switch {
	case !exact:
		pinNote = fmt.Sprintf(" The sidecar reference names no release tag (jax-<version>, <yyyymmdd>-jax_<version> or latest), so gcluster uses the latest release; pass %s to pin the images.", flags)
	case pinnedByDigest:
		pinNote = fmt.Sprintf(" The sidecar reference is pinned by digest, so container runtimes pull that digest instead of the %q tag; pass %s to pin matching images.", tag, flags)
	}
	if len(kept) > 0 {
		target := "that image"
		if len(chosen) > 1 {
			target = "those images"
		}
		tail := ""
		if pinNote == "" {
			tail = ", or pass " + flags + " yourself"
		}
		msg += fmt.Sprintf(" It does not derive %s from the %s you passed, so make sure the images work together%s.", target, strings.Join(kept, " and "), tail)
	}
	warn := len(kept) > 0 || pinNote != ""
	if pinNote != "" {
		msg += pinNote
	} else if !warn {
		msg += fmt.Sprintf(" Pass %s to use other images.", flags)
	}
	return msg + " See " + pathwaysSidecarImagesDoc + ".", warn
}

// customSidecarNotice returns the warning for a sidecar gcluster cannot match to a Pathways release when it keeps its
// default server or proxy image, or "" when the user passed every image the JobSet runs.
func customSidecarNotice(p orchestrator.PathwaysJobDefinition) string {
	var defaulted []string
	if p.ServerImage == "" {
		defaulted = append(defaulted, "server image "+defaultPathwaysServerImage)
	}
	if !p.DeploySPS && p.ProxyServerImage == "" {
		defaulted = append(defaulted, "proxy image "+defaultPathwaysProxyImage)
	}
	if len(defaulted) == 0 {
		return ""
	}
	subject := "server and proxy images"
	if p.DeploySPS {
		subject = "server image"
	}
	_, flags := pathwaysImageNouns(p.DeploySPS)
	return fmt.Sprintf("Using gcluster's default Pathways %s with the colocated Python sidecar %s. The %s should ideally come from the same Pathways release as the sidecar's base image, and gcluster cannot verify before submission that the images work together, so please make sure the images you run are correct; for example, a sidecar built from %s needs %s from %s with the same tag. See %s.",
		strings.Join(defaulted, " and "), p.ColocatedPythonSidecarImage, subject, publicPathwaysSidecarImage, flags, publicPathwaysImageRepo, pathwaysSidecarImagesDoc)
}

// activeWorkloadImages returns the distinct images the rendered pods pull: the workload image, the ML
// Diagnostics telemetry sidecar image when enabled, and, for Pathways, the server, proxy, worker, and
// colocated sidecar images of the selected deployment mode.
func activeWorkloadImages(job orchestrator.JobDefinition, fullImageName, telemetryImage string) []string {
	raw := []string{fullImageName}
	if job.IsPathwaysJob {
		pw := job.Pathways
		if pw.Headless || pw.DeploySPS {
			raw = nil
		}
		if pw.RunSPSWorkload == "" {
			raw = append(raw, pw.ServerImage, pw.WorkerImage, pw.ColocatedPythonSidecarImage)
		}
		if !pw.DeploySPS {
			raw = append(raw, pw.ProxyServerImage)
		}
	}
	raw = append(raw, telemetryImage)
	var images []string
	for _, img := range raw {
		if img != "" && !slices.Contains(images, img) {
			images = append(images, img)
		}
	}
	return images
}

// ambientTelemetryImage returns GCLUSTER_TELEMETRY_IMAGE unless --ml-diagnostics-image was given explicitly
// (an explicit empty value opts out of the environment default, mirroring --image-pull-secret="").
func ambientTelemetryImage(job orchestrator.JobDefinition) string {
	if job.MLDiagnosticsImageSpecified {
		return ""
	}
	return k8s.TelemetryImageFromEnv()
}
