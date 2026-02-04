echo "=== BrainNav SSL Certificate Generation with OpenSSL ==="

CERTS_DIR="./certs"
if [ ! -d "$CERTS_DIR" ]; then
    mkdir -p "$CERTS_DIR"
    echo "Created certs directory"
fi

if ! command -v openssl &> /dev/null; then
    echo "ERROR: OpenSSL not found. Please install OpenSSL first!"
    echo "Ubuntu/Debian: sudo apt-get install openssl"
    echo "CentOS/RHEL: sudo yum install openssl"
    echo "macOS: brew install openssl"
    exit 1
fi

DOMAIN="localhost"
COUNTRY="ID"
STATE="Jakarta"
CITY="Jakarta"
ORG="BrainNav"
ORG_UNIT="Development"
EMAIL="admin@brainnav.local"
VALID_DAYS=365

echo "OpenSSL version: $(openssl version)"
echo "Generating SSL certificate for domain: $DOMAIN"

echo "Step 1: Generating private key..."
openssl genrsa -out "$CERTS_DIR/server.key" 2048

if [ $? -ne 0 ]; then
    echo "ERROR: Failed to generate private key"
    exit 1
fi

echo "Step 2: Creating certificate signing request..."
CSR_SUBJECT="/C=$COUNTRY/ST=$STATE/L=$CITY/O=$ORG/OU=$ORG_UNIT/CN=$DOMAIN/emailAddress=$EMAIL"

openssl req -new -key "$CERTS_DIR/server.key" -out "$CERTS_DIR/server.csr" -subj "$CSR_SUBJECT"

if [ $? -ne 0 ]; then
    echo "ERROR: Failed to create CSR"
    exit 1
fi

cat > "$CERTS_DIR/server.ext" << EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
DNS.2 = brainnav.local
DNS.3 = *.brainnav.local
IP.1 = 127.0.0.1
IP.2 = ::1
EOF

echo "Step 3: Generating self-signed certificate..."
openssl x509 -req -in "$CERTS_DIR/server.csr" -signkey "$CERTS_DIR/server.key" -out "$CERTS_DIR/server.crt" -days $VALID_DAYS -extensions v3_req -extfile "$CERTS_DIR/server.ext"

if [ $? -ne 0 ]; then
    echo "ERROR: Failed to generate certificate"
    exit 1
fi

rm -f "$CERTS_DIR/server.csr"
rm -f "$CERTS_DIR/server.ext"

echo "Step 4: Verifying certificate..."
openssl x509 -in "$CERTS_DIR/server.crt" -text -noout | grep -E "Subject:|DNS:|IP Address:|Not Before:|Not After:"

echo "Step 5: Setting file permissions..."
chmod 600 "$CERTS_DIR/server.key"  
chmod 644 "$CERTS_DIR/server.crt"  

echo ""
echo "=== SSL Certificate Generation Complete! ==="
echo "Certificate: $CERTS_DIR/server.crt"
echo "Private Key: $CERTS_DIR/server.key"
echo ""
echo "Certificate Details:"
echo "- Domain: $DOMAIN"
echo "- Valid for: $VALID_DAYS days"
echo "- Algorithm: RSA 2048-bit"
echo "- Subject Alternative Names: localhost, brainnav.local, *.brainnav.local"
echo ""
echo "⚠️  IMPORTANT NOTES:"
echo "1. This is a self-signed certificate for development use only"
echo "2. Browsers will show security warnings - this is normal"
echo "3. For production, use a certificate from a trusted CA"
echo "4. Add 'brainnav.local' to your hosts file if needed"
echo ""
echo "Your BrainNav application will now run on HTTPS! 🔒"
