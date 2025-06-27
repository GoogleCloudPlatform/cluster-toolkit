// Sidebar toggle functionality
document.addEventListener('DOMContentLoaded', function() {
    const sidebarToggle = document.getElementById('sidebarToggle');
    const sidebar = document.getElementById('sidebar');
    const mainContent = document.getElementById('mainContent');

    if (!sidebarToggle || !sidebar || !mainContent) {
        console.warn('Sidebar elements not found');
        return;
    }

    // Load saved state
    const sidebarCollapsed = localStorage.getItem('sidebarCollapsed') === 'true';
    if (sidebarCollapsed) {
        sidebar.classList.add('sidebar-collapsed');
        mainContent.classList.add('main-content-expanded');
    }

    // Toggle sidebar
    sidebarToggle.addEventListener('click', function() {
        sidebar.classList.toggle('sidebar-collapsed');
        mainContent.classList.toggle('main-content-expanded');

        // Save state
        localStorage.setItem('sidebarCollapsed', sidebar.classList.contains('sidebar-collapsed'));
    });
});

// Set active navigation tab
function setActiveTab(tabId) {
    const tab = document.getElementById(tabId);
    if (tab) {
        tab.classList.add('active');
    }
} 