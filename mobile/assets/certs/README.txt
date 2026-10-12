Optional private CA of the installation (MOB-8)
================================================

Dart's HTTP client (used for the API and Keycloak) trusts only the public
system CAs; it ignores CAs installed in Android settings. If the server uses
the private CA from `make certs`, copy that CA here before building:

    cp infra/nginx/certs/securepatrol-ca.crt mobile/assets/certs/securepatrol-ca.crt
    flutter build apk --release

The file name must be exactly `securepatrol-ca.crt` (PEM). It is git-ignored:
CI can write it from a secret (e.g. SECUREPATROL_CA_PEM) before the build.
Without it the app trusts public CAs only (e.g. a Let's Encrypt certificate).
