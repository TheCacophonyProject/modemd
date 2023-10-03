#!/bin/bash

# Check that dnsutils is installed
if ! command -v nslookup &> /dev/null; then
  echo "nslookup not found, can be installed with: sudo apt install dnsutils"
  exit 1
fi

systemctl daemon-reload
systemctl enable modemd.service
systemctl restart modemd.service
