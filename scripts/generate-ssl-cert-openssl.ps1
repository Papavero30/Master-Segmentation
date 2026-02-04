Write-Host "=== BrainNav SSL Certificate Generation with OpenSSL ===" -ForegroundColor Green

$certsDir = ".\certs"
if (!(Test-Path $certsDir)) {
    New-Item -ItemType Directory -Path $certsDir
    Write-Host "Created certs directory" -ForegroundColor Yellow
}

try {
    $opensslVersion = & openssl version
    Write-Host "OpenSSL found: $opensslVersion" -ForegroundColor Green
} catch {
    Write-Host "ERROR: OpenSSL not found. Please install OpenSSL first!" -ForegroundColor Red
    Write-Host "Download from: https://slproweb.com/products/Win32OpenSSL.html" -ForegroundColor Yellow
    exit 1
}

$domain = "localhost"
$country = "ID"
$state = "Jakarta"
$city = "Jakarta"
$org = "BrainNav"
$orgUnit = "Development"
$email = "admin@brainnav.local"
$validDays = 365

Write-Host "Generating SSL certificate for domain: $domain" -ForegroundColor Cyan

Write-Host "Step 1: Generating private key..." -ForegroundColor Yellow
& openssl genrsa -out "$certsDir\server.key" 2048

if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: Failed to generate private key" -ForegroundColor Red
    exit 1
}

Write-Host "Step 2: Creating certificate signing request..." -ForegroundColor Yellow
$csrSubject = "/C=$country/ST=$state/L=$city/O=$org/OU=$orgUnit/CN=$domain/emailAddress=$email"

& openssl req -new -key "$certsDir\server.key" -out "$certsDir\server.csr" -subj $csrSubject

if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: Failed to create CSR" -ForegroundColor Red
    exit 1
}

$extFile = @"
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
"@

$extFile | Out-File -FilePath "$certsDir\server.ext" -Encoding ASCII

Write-Host "Step 3: Generating self-signed certificate..." -ForegroundColor Yellow
& openssl x509 -req -in "$certsDir\server.csr" -signkey "$certsDir\server.key" -out "$certsDir\server.crt" -days $validDays -extfile "$certsDir\server.ext"

if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: Failed to generate certificate" -ForegroundColor Red
    exit 1
}

Remove-Item "$certsDir\server.csr" -ErrorAction SilentlyContinue
Remove-Item "$certsDir\server.ext" -ErrorAction SilentlyContinue

Write-Host "Step 4: Verifying certificate..." -ForegroundColor Yellow
& openssl x509 -in "$certsDir\server.crt" -text -noout | Select-String -Pattern "Subject:|DNS:|IP Address:|Not Before:|Not After:"

Write-Host "Step 5: Setting file permissions..." -ForegroundColor Yellow
icacls "$certsDir\server.key" /inheritance:r /grant:r "$env:USERNAME:(R)" /T
icacls "$certsDir\server.crt" /inheritance:r /grant:r "$env:USERNAME:(R)" /T

Write-Host ""
Write-Host "=== SSL Certificate Generation Complete! ===" -ForegroundColor Green
Write-Host "Certificate: $certsDir\server.crt" -ForegroundColor Cyan
Write-Host "Private Key: $certsDir\server.key" -ForegroundColor Cyan
Write-Host ""
Write-Host "Certificate Details:" -ForegroundColor Yellow
Write-Host "- Domain: $domain" -ForegroundColor White
Write-Host "- Valid for: $validDays days" -ForegroundColor White
Write-Host "- Algorithm: RSA 2048-bit" -ForegroundColor White
Write-Host "- Subject Alternative Names: localhost, brainnav.local, *.brainnav.local" -ForegroundColor White
Write-Host ""
Write-Host "⚠️  IMPORTANT NOTES:" -ForegroundColor Red
Write-Host "1. This is a self-signed certificate for development use only" -ForegroundColor Yellow
Write-Host "2. Browsers will show security warnings - this is normal" -ForegroundColor Yellow
Write-Host "3. For production, use a certificate from a trusted CA" -ForegroundColor Yellow
Write-Host "4. Add 'brainnav.local' to your hosts file if needed" -ForegroundColor Yellow
Write-Host ""
Write-Host "Your BrainNav application will now run on HTTPS! [SSL ENABLED]" -ForegroundColor Green
