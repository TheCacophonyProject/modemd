#!/bin/bash

# Check that dnsutils is installed
if ! command -v nslookup &> /dev/null; then
  echo "nslookup not found, can be installed with: sudo apt install dnsutils"
  exit 1
fi

udevadm control --reload-rules

systemctl daemon-reload
systemctl enable modemd.service

# Using start instead of restart to avoid restarting the service if it's already running, causing the internet to turn off potentially
# At the end of a salt update it will restart the service if it was updated.
systemctl restart modemd.service
