# Multi-platform HPC Application System

This is a web front-end for HPC applications on GCP. The system enables admin users to manage the life cycles of HPC clusters and install applications. It also allows users to prepare and submit HPC jobs, and run benchmarks. This application is built upon the Django framework.

### Deployment

This system can be deployed on GCP using the following procedures.

A user is handed with the deployment script `deploy.sh` together with deployment key files that grant access to some private GitHub repositories. In the future, when the system becomes mature enough and is made public, the key files will no longer be necessary.

Run `deploy.sh` interactively on any Linux based client machines, or from a GCP cloud shell. The script will prompt to collect the following information from the user:

* Login details for the superuser of the web application.
* Name, region and instance type of the compute engine virtual machine to host the web application.
* An SSH key for the admin user to gain access to the server.

The hosting VM will then be created and configured automatically. The whole process takes approxiately 15 minutes.  

### Post-deployment Configuations

After the deployment, the web application requires some additional configurations.

  - The site must be associated with a domain name. This is required to allow user access via Google authentication.
  - For a production deployment, an SSL certificate should be obtained for the domain to support secure connections.
  - The deployment script created a firewall rule to restrict access to the site from the Internet. Relax or remove the firewall rule for production.
  - After setting up the domain name, visit the hosting GCP project and register this web application (in GCP console, create an OAuth 2.0 credential under the *APIs and Services* section). Update the *Authorised JavaScript origins* to the full domain name and *Authorised redirect URIs* fields to `<DOMAIN_NAME>/accounts/google/login/callback/`. Note the *Client ID* and *Client secret*.
  - SSH into the server and edit the Django settings at `/opt/gcluster/c398_MultiCloud_Benchmarking/website/website/settings.py` by adding the domain name to the *ALLOWED_HOSTS* list.
  - The website should be fully functional now. Open it in a browser and log in using the superuser account created earlier. Update the Django social application database table at `https://<DOMAIN_NAME>/admin/socialaccount/socialapp/1/change/`, replacing the two PLACEHOLDERs by the *Client ID* and *Client secret* to complete the set up.

