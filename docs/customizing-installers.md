# Customizing installers

Elemental installer images can be customized using the `elemental3 customize` command.

The following command takes an input (./build/installer.iso) and generates a
new installer2.iso in the output directory including a new config-script, an
overlay and some extra kernel cmdline parameters:

```sh
./build/elemental3 customize \
    --input ./build/installer.iso
    --output ./build
    --name installer2
    --cmdline="console=ttyS0"
    --config ./examples/elemental/install/config.sh
    --overlay tar:./build/overlays.tar.gz
```
