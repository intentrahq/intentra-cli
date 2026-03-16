#!/bin/sh
set -e

GITHUB_OWNER="intentrahq"
GITHUB_REPO="intentra-cli"
BINARY_NAME="intentra"
INSTALL_DIR="/usr/local/bin"

main() {
    check_dependencies
    detect_platform
    get_latest_version
    download_and_install
    verify_installation
    print_success
}

check_dependencies() {
    if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
        echo "Error: curl or wget is required to download intentra"
        exit 1
    fi

    if ! command -v tar >/dev/null 2>&1; then
        echo "Error: tar is required to extract the archive"
        exit 1
    fi
}

detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$OS" in
        darwin)
            OS="darwin"
            ;;
        linux)
            OS="linux"
            ;;
        mingw*|msys*|cygwin*)
            echo "Error: Windows detected. Please use PowerShell installer:"
            echo "  irm https://install.intentra.sh/install.ps1 | iex"
            exit 1
            ;;
        *)
            echo "Error: Unsupported operating system: $OS"
            exit 1
            ;;
    esac

    case "$ARCH" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        arm64|aarch64)
            ARCH="arm64"
            ;;
        *)
            echo "Error: Unsupported architecture: $ARCH"
            exit 1
            ;;
    esac

    echo "Detected platform: ${OS}/${ARCH}"
}

get_latest_version() {
    echo "Fetching latest version..."
    
    RELEASES_URL="https://api.github.com/repos/${GITHUB_OWNER}/${GITHUB_REPO}/releases/latest"
    
    if command -v curl >/dev/null 2>&1; then
        VERSION=$(curl -sL "$RELEASES_URL" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    else
        VERSION=$(wget -qO- "$RELEASES_URL" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    fi
    
    if [ -z "$VERSION" ]; then
        echo "Error: Could not determine latest version"
        exit 1
    fi
    
    VERSION_NUM="${VERSION#v}"
    echo "Latest version: ${VERSION}"
}

download_and_install() {
    ARCHIVE_NAME="${BINARY_NAME}_${VERSION_NUM}_${OS}_${ARCH}.tar.gz"
    DOWNLOAD_URL="https://github.com/${GITHUB_OWNER}/${GITHUB_REPO}/releases/download/${VERSION}/${ARCHIVE_NAME}"
    CHECKSUM_URL="https://github.com/${GITHUB_OWNER}/${GITHUB_REPO}/releases/download/${VERSION}/checksums.txt"
    
    TMP_DIR=$(mktemp -d)
    trap 'rm -rf "$TMP_DIR"' EXIT
    
    echo "Downloading ${ARCHIVE_NAME}..."
    if command -v curl >/dev/null 2>&1; then
        curl -sL "$DOWNLOAD_URL" -o "${TMP_DIR}/${ARCHIVE_NAME}"
        curl -sL "$CHECKSUM_URL" -o "${TMP_DIR}/checksums.txt"
    else
        wget -q "$DOWNLOAD_URL" -O "${TMP_DIR}/${ARCHIVE_NAME}"
        wget -q "$CHECKSUM_URL" -O "${TMP_DIR}/checksums.txt"
    fi
    
    echo "Verifying checksum..."
    cd "$TMP_DIR"
    EXPECTED_CHECKSUM=$(grep "${ARCHIVE_NAME}" checksums.txt | awk '{print $1}')
    
    if [ -z "$EXPECTED_CHECKSUM" ]; then
        echo "Warning: Could not find checksum for ${ARCHIVE_NAME}, skipping verification"
    else
        if command -v sha256sum >/dev/null 2>&1; then
            ACTUAL_CHECKSUM=$(sha256sum "${ARCHIVE_NAME}" | awk '{print $1}')
        elif command -v shasum >/dev/null 2>&1; then
            ACTUAL_CHECKSUM=$(shasum -a 256 "${ARCHIVE_NAME}" | awk '{print $1}')
        else
            echo "Warning: No sha256sum or shasum available, skipping checksum verification"
            ACTUAL_CHECKSUM="$EXPECTED_CHECKSUM"
        fi
        
        if [ "$EXPECTED_CHECKSUM" != "$ACTUAL_CHECKSUM" ]; then
            echo "Error: Checksum verification failed"
            echo "Expected: $EXPECTED_CHECKSUM"
            echo "Actual:   $ACTUAL_CHECKSUM"
            exit 1
        fi
        echo "Checksum verified"
    fi
    
    echo "Extracting archive..."
    tar -xzf "${ARCHIVE_NAME}"
    
    echo "Installing to ${INSTALL_DIR}..."
    if [ -w "$INSTALL_DIR" ]; then
        mv "${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
        chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    else
        echo "Need sudo to install to ${INSTALL_DIR}"
        sudo mv "${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
        sudo chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    fi
}

verify_installation() {
    if ! command -v "$BINARY_NAME" >/dev/null 2>&1; then
        if [ -x "${INSTALL_DIR}/${BINARY_NAME}" ]; then
            echo ""
            echo "Warning: ${BINARY_NAME} was installed but is not in your PATH"
            echo "Add ${INSTALL_DIR} to your PATH, or run:"
            echo "  export PATH=\"\$PATH:${INSTALL_DIR}\""
            return
        fi
        echo "Error: Installation verification failed"
        exit 1
    fi
}

print_success() {
    echo ""
    echo "============================================"
    echo "  Intentra CLI installed successfully!"
    echo "============================================"
    echo ""
    echo "Version: ${VERSION}"
    echo "Location: ${INSTALL_DIR}/${BINARY_NAME}"
    echo ""
    echo "Get started:"
    echo "  intentra --help"
    echo "  intentra login"
    echo "  intentra install cursor"
    echo ""
    echo "Documentation: https://intentra.sh/docs"
    echo ""
}

main "$@"
