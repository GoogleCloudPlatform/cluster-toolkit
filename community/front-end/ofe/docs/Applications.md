# Cluster Toolkit FrontEnd - Application Installation Guide

<!--
0        1         2         3         4         5         6         7        8
1234567890123456789012345678901234567890123456789012345678901234567890234567890
-->

Administrators can install and manage applications in TKFE in the following
ways:

## Install Spack applications

The recommended method of application installation is via
[Spack](https://spack.readthedocs.io). Spack, an established package
management system for HPC, contains build recipes of the most widely used
open-source HPC applications. This method is completed automated. Spack
installation is performed as a Slurm job. Simply choose a Slurm partition to
run Spack.

Advanced users may also customise the installation by specifying a Spack spec
string.

## Install custom applications

For applications not yet covered by the Spack package repository, e.g., codes
developed in-house, or those failed to build by Spack, use custom
installations by specifying custom scripts containing steps to build the
applications.

## Register manually installed applications

Complex packages, such as some commercial applications that may require
special steps to set up, can be installed manually on the cluster's shared
filesystem. Once done, they can be registered with the FrontEnd so that future
job submissions can be automated through the FrontEnd.

---

## Application status

Clicking the *Applications* item in the main menu leads to the application
list page which displays all existing application installations. Applications
can be in different states and their *Actions* menus adapt to this information
to show different actions:

- `n` - Application is being newly configured by an admin user through the web
  interface. At this stage, only a database record exists in the system. The
  user is free to edit this application, although in the case of a Spack
  application  , most information is automatically populated. When ready,
  clicking Spack Install from the Actions menu to initiate the installation
  process.
- `p` - Application is being prepared. In this state, application build is
  triggered from the web interface and information is being passed to the
  cluster.
- `q` - In this state the Slurm job for building this application is queueing
  on the target cluster. Note that all application installations are performed
  on a compute node. This leaves the relatively lightweight controller and
  login nodes to handle management tasks only, and also ensures the maximum
  possible compatibility between the generated binary and the hardware to run
  it in the future.
- `i` - In this state, the Slurm job for building this application is running
  on the target cluster. Spack is fully responsible for building this
  application and managing its dependencies.
- `r` - Spack build has completed successfully, and the application is ready
   to run by authorised users on the target cluster.
- `e` - Spack has somehow failed to build this application. Refer to the
  debugging section of this document on how to debug a failed installation.
- `x` - If a cluster has been destroyed, all applications on this cluster will
  be marked in this status. Destroying a cluster won’t affect the application
  and job records stored in the database.

A visual indication is shown on the website for any application installation
in progress. Also, the relevant web pages will refresh every 15 seconds to
pick status changes.

### Install a Spack application

A typical workflow for installing a new Spack application is as follows:

1. From the application list page, press the *New application* button. In the
   next form, select the target cluster and choose *Spack installation*.
1. In the *Create a new Spack application* form, type a keyword in the
   *Create a new Spack application* form, and use the auto-completion function
   to choose the Spack package to install. The *Name* and *Version* fields are
   populated automatically. If Spack supports multiple versions of the
   application, click the drop-down list there to select the desired version.
1. Spack supports variants - applications built with customised compile-time
   options. These may be special compiler flags or optional features that must
   be switched on manually. Advanced users may supply additional specs using
   the optional *Spack spec* field.
   - For a guide to the Spack spec syntax see the [Spack documentation](https://spack.readthedocs.io/en/latest/basic_usage.html#building-a-specific-version)
   - By default, the GCC 11.2 compiler is used for building all applications.
   - Other compilers may be specified with the `%` compiler specifier and an
     optional version number using the `@` version specifier (e.g.,
     `%intel@19.1.1.217`). Obviously, admin users are responsible for
     installing and configuring those additional compilers and, if
     applicable, arrange their licenses.
   - Spack is configured in this system to use Intel MPI to build
     application.
   - Other MPI libraries may be specified with the ^ dependency specifier and
     an optional version number.
1. The Description field is populated automatically from the information found
   in the Spack repository.
1. Choose an Slurm partition from the drop-down list to run the Slurm job for
   application installation. This, typically, should be the same partitions to
   run the application in the future.
1. Click the *Save* button. A database record is then created for this
   application in the system. On the next page, click the *Edit* button to
   modify the application settings; click the *Delete* button to delete this
   record if desired; click the *Spack install* button to actually start
   building this application on the cluster. The last step can take quite a
   while to complete depending on the application. A visual indication is
   given on the related web pages until the installation Slurm job is
   completed.
1. A successfully installed application will have its status updated to
  ‘ready’. A *New Job* button becomes available from the Actions menu on the
   application list page, or from the application detail page. The
   [User Guide](user_guide.md) contains additional information on how jobs can
   be prepared and submitted.

---

## Application problems

Spack installation is fairly reliable. However, there are thousands of
packages in the Spack repository and packages are not always tested on all
systems. If a Spack installation returns an error, first locate the Spack logs
by clicking the *View Logs* button from the application detail page. Then
identify from the *Installation Error Log* the root cause of the problem.

Spack installation problems can happen with not only the package installed,
but also its dependencies. There is no general way to debug Spack compilation
problems. It may be helpful submit an interactive job to the cluster and debug
Spack problems there manually. It is recommended to not build applications
from the controller or login nodes, as the underlying processor may differ to
the compute nodes.

Complex bugs should be reported to Spack. If an easy fix can be found, note
the procedure. This can be then used in a custom installation.
