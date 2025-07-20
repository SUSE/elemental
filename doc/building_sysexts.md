## How to build systemd sysextensions using mkosi


### Prerequisites

building a sytemd sysextension for use with elemental image requires these assets:
* mkosi
* a set of mkosi.conf configuration files and assets

For the purpose of this documentation, we are assuming that openSUSE Tumbleweed is being used. The [tools-sysext example directory(../examples/tools-sysext/) includes an example set of configurations that can be used.


### Creating a sysextension

There are 3 `mkosi.conf` configurations needed

* [mkosi.conf in the base directory](../examples/tools-sysext/mkosi.conf)
* [base/mkosi.conf defining the base layer](../examples/tools-sysext/mkosi.images/base/mkosi.conf)
* [tools/mkosi.conf defining the extension layer](../examples/tools-sysext/mkosi.images/tools/mkosi.conf)

The systemd extension is created by "subtracing" the tools layer from the base layer. The base layer hence needs to include all the files that are assumed to be available on the host operating system, and the tools definition defines the extensions over that.

Building the systemd extension can be achieved by invoking

`cd examples/tools-sysext && mkosi --directory $PWD`

This will produce the base and extension images and assemble it into a systemd extension.

