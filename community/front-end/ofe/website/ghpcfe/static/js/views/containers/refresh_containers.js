/**
 * Copyright 2026 "Google LLC"
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// refresh_containers.js

import { refreshTableData, startAutoRefresh } from "/static/js/views/shared/filter_and_refresh.js";

export function refreshContainers(registryId) {
    fetch(`/registry/${registryId}/containers/`)
        .then(response => response.text())
        .then(html => {
            document.getElementById("image-list").innerHTML = html;
        })
        .catch(error => {
            console.error("Error refreshing containers:", error);
        });
}

export function startContainerAutoRefresh(registryId) {
    setInterval(() => refreshContainers(registryId), 10000);
}
