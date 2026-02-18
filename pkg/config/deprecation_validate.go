/**
 * Copyright 2026 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package config

import (
	"fmt"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/modulereader"
	"strings"
	"time"
)

func validateDeprecation(modID ModuleID, info modulereader.ModuleInfo) error {
	deprecationDateStr := info.Metadata.Ghpc.DeprecationDate
	if deprecationDateStr == "" {
		return nil // Not deprecated, no warning needed
	}

	deprecationDate, err := time.Parse("2006-01-02", deprecationDateStr)
	if err != nil {
		return fmt.Errorf("The module %q has a malformed deprecation_date: %q, expected format is: YYYY-MM-DD.", modID, deprecationDateStr)
	}

	currentTime := time.Now()
	alternativeModule := info.Metadata.Ghpc.AlternativeModule

	var msgBuilder strings.Builder
	if currentTime.Before(deprecationDate) {
		// Phase 1: Announcement & Warning Period
		msgBuilder.WriteString(fmt.Sprintf(`The module %s will be deprecated on %s. Module will be removed on this date.
No new features will be added to the module. Bug fixes will be avoided, unless absolutely critical. No new blueprints should use this module.`,
			modID, deprecationDate.Format("2006-01-02")))
	} else {
		// Phase 2: Past Deprecation Date (Module Removed)
		msgBuilder.WriteString(fmt.Sprintf("The module %s was deprecated on %s and no more support is available.", modID, deprecationDate.Format("2006-01-02")))
	}
	if alternativeModule != "" {
		msgBuilder.WriteString(fmt.Sprintf("\nPlease plan your migration to %s.", alternativeModule))
	}
	logging.Warn("%s", msgBuilder.String())
	return nil
}
