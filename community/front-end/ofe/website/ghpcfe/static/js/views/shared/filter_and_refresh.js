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

// filter_and_refresh.js

// Filter table rows based on the selected dropdown value.
// It always gets the latest rows from the table.
export function setupDropdownFilter(dropdownId, tableId, dataAttribute) {
  const dropdown = document.getElementById(dropdownId);
  if (!dropdown) return;

  // When the dropdown value changes, update the table rows display.
  dropdown.addEventListener("change", function () {
    const selectedValue = this.value;
    const tableRows = document.querySelectorAll(`#${tableId} tr`);
    tableRows.forEach(row => {
      const rowValue = row.getAttribute(`data-${dataAttribute}`);
      row.style.display = (selectedValue === "all" || rowValue === selectedValue) ? "" : "none";
    });
    // Optionally, save the current selection (for example, to localStorage) so you can reapply it later.
    localStorage.setItem("selectedCluster", selectedValue);
  });

  // On page load, if there is a saved filter, apply it.
  const savedValue = localStorage.getItem("selectedCluster");
  if (savedValue) {
    dropdown.value = savedValue;
    dropdown.dispatchEvent(new Event("change"));
  }
}

// Refresh table data from a URL and reapply the filter afterwards.
export function refreshTableData(url, tableId, renderRowCallback) {
  fetch(url)
    .then(response => response.json())
    .then(data => {
      const tableBody = document.getElementById(tableId);
      if (!tableBody) return;
      tableBody.innerHTML = ""; // Clear old rows
      data.forEach(item => {
        tableBody.innerHTML += renderRowCallback(item);
      });
      // Reapply the current filter.
      const dropdown = document.getElementById("cluster-select");
      if (dropdown) {
        dropdown.dispatchEvent(new Event("change"));
      }
    })
    .catch(error => console.error("Error refreshing table data:", error));
}

// Simple auto-refresh that calls a callback at the specified interval.
export function startAutoRefresh(callback, interval = 10000) {
  setInterval(callback, interval);
}
