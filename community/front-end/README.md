# HPC Frontend

This is a web front-end for HPC applications on GCP. It delegates to the Google HPC Toolkit to create cloud resources for HPC clusters. Through the convenience of web interface, system administrators can manage the life cycles of HPC clusters and install applications; users can prepare & submit HPC jobs and run benchmarks. This web application is built upon the Django framework.

## Deployment

This system can be deployed on GCP using the following procedures.

A user is handed with the deployment script `deploy.sh` together with deployment key files that grant access to some private GitHub repositories. In the future, when the system becomes mature enough and is made public, the key files will no longer be necessary.

Run `deploy.sh` interactively on any Linux based client machines, or from a GCP cloud shell. The script will prompt to collect the following information from the user:

* A hosting GCP project.
* Login details for the superuser of the web application.
* Name, region, and instance type of the compute engine virtual machine to host the web application.
* A domain name to be used by the system for a production deployment (a test site can run on an IP address but will not be fully functioning).

The hosting VM will then be created by Terraform and configured automatically. The whole process takes approximately 15 minutes.  

## Post-deployment Configurations

After the deployment, the web application requires some additional configurations.

* The site must be associated with a domain name. This is required to allow user access via Google authentication.
* For a production deployment, an SSL certificate should be obtained for the domain to support secure connections. The deployment script will attempt to obtain a Let's Encrypt certificate if sufficient information is supplied. Otherwise, admin users can set this up later.
* To enable Google authentication, after setting up the domain name, visit the hosting GCP project and register this web application (in GCP console, create an OAuth 2.0 credential under the *APIs and Services* section). Update the *Authorised JavaScript origins* to the full domain name and *Authorised redirect URIs* fields to `<DOMAIN_NAME>/accounts/google/login/callback/`. Note the *Client ID* and *Client secret*.
* The website should be up and running now. Open it in a browser and log in using the superuser account created earlier. Update the Django social application database table at `https://<DOMAIN_NAME>/admin/socialaccount/socialapp/1/change/`, replacing the two PLACEHOLDERs by the *Client ID* and *Client secret* to complete the set up.
