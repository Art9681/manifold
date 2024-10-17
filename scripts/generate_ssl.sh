#!/bin/bash

DATAPATH="$(eval echo ~$USER)/.manifold/ssl"

# Check of the DATAPATH exists, otherwise create it
if [ ! -d "$DATAPATH" ]; then
  mkdir -p $DATAPATH
fi

# Define the server name and certificate file paths
SERVER_NAME="m3.manifold.local"
CERT_DIR="$DATAPATH/ssl"
PRIVATE_KEY="$CERT_DIR/server.key"
CERTIFICATE="$CERT_DIR/server.crt"

# Create the directory for SSL certificates if it doesn't exist
mkdir -p $CERT_DIR

# Generate the private key and self-signed certificate
openssl req -x509 -nodes -days 365 -newkey rsa:4096 \
  -keyout $PRIVATE_KEY -out $CERTIFICATE \
  -subj "/CN=$SERVER_NAME"

# Print a message indicating that the certificate has been generated
echo "Self-signed SSL certificate generated for $SERVER_NAME"
echo "Private Key: $PRIVATE_KEY"
echo "Certificate: $CERTIFICATE"
