"""website URL Configuration

The `urlpatterns` list routes URLs to views. For more information please see:
    https://docs.djangoproject.com/en/3.1/topics/http/urls/
Examples:
Function views
    1. Add an import:  from my_app import views
    2. Add a URL to urlpatterns:  path('', views.home, name='home')
Class-based views
    1. Add an import:  from other_app.views import Home
    2. Add a URL to urlpatterns:  path('', Home.as_view(), name='home')
Including another URLconf
    1. Import the include() function: from django.urls import include, path
    2. Add a URL to urlpatterns:  path('blog/', include('blog.urls'))
"""
from django.contrib import admin
from django.urls import path, include
from django.conf import settings
from django.conf.urls.static import static

urlpatterns = [
    path("admin/", admin.site.urls),
]

# add paths from the ghpcfe application
urlpatterns += [
    path("", include("ghpcfe.urls")),
]

# add url mapping to serve static files

urlpatterns += static(settings.STATIC_URL, document_root=settings.STATIC_ROOT)

# customise admin site
admin.site.site_header = "HPC Toolkit FrontEnd Admin Site"
admin.site.index_title = "Administration"
admin.site.site_title = "Administration site"

# add Django site authentication urls (for login, logout, password management)
urlpatterns += [
    path("accounts/", include("django.contrib.auth.urls")),
    path("accounts/", include("allauth.urls")),
]
