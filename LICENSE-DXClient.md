# HCL DXClient in the operator image

The source code of this repository is MIT licensed (see [LICENSE](LICENSE)).

The **container image** of the operator (`ghcr.io/europesip/hcl-dx-scriptapp-operator`) is built on top of
HCL's public DXClient image (`hclcr.io/dx-public/dxclient`), so it **contains HCL DXClient**, which is
**HCL software under its own licence** (an HCL licence for non-warranted programs), not under MIT. The full
text is in the image, in `/dxclient/LICENSE`, and DXClient prints it with:

```bash
docker run --rm hclcr.io/dx-public/dxclient:v95_CF239_20261103-1404 /dxclient/bin/dxclient accept-license
```

- The operator does not run DXClient unless you accept that licence: the Helm chart refuses to install
  without `dxclient.acceptLicense=true`, and the binary checks `DXCLIENT_ACCEPT_LICENSE=true`.
- DXClient is used as HCL distributes it, unmodified.
- To use another DXClient version (for example the one of your DX level), build the image with
  `--build-arg DXCLIENT_IMAGE=hclcr.io/dx-public/dxclient:<tag>`.

HCL, HCL Digital Experience, HCL DX Compose and DXClient are trademarks of HCL Technologies Ltd. This project
is not an HCL product and is not supported by HCL.
