#!/bin/sh
sudo mkdir -p /opt/cni/bin
curl -sSL https://github.com/containernetworking/plugins/releases/download/v0.9.1/cni-plugins-linux-amd64-v0.9.1.tgz | sudo tar -xz -C /opt/cni/bin
ls /opt/cni/bin

