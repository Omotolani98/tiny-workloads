#!/bin/bash

# Exit immediately if a command exits with a non-zero status.
set -e

REPO="Omotolani98/tiny-workloads" # Updated to tiny-workloads repository
BINARY_NAME="tiny-workloads" # Updated to tiny-workloads binary name
VERSION="${1:-latest}"

echo "🚀 Starting installation of $BINARY_NAME..."

# --- Check for necessary commands ---
echo "🔍 Checking for required tools..."
if ! command -v curl &> /dev/null; then
    echo "❌ Error: curl is not installed. Please install curl to proceed."
    exit 1
fi
if ! command -v tar &> /dev/null; then
    echo "❌ Error: tar is not installed. Please install tar to proceed."
    exit 1
fi
echo "✅ Required tools found."

# --- Detect platform ---
echo "🌍 Detecting platform..."
ARCH=$(uname -m)
OS=$(uname -s | tr '[:upper:]' '[:lower:]')

case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64 | arm64) ARCH="arm64" ;;
  *) echo "❌ Unsupported architecture: $ARCH"; exit 1 ;;
esac

echo "✅ Detected OS: $OS, Architecture: $ARCH."

# --- Determine version ---
if [ "$VERSION" = "latest" ]; then
  echo "✨ Fetching latest release version..."
  VERSION=$(curl -s https://api.github.com/repos/${REPO}/releases/latest | grep tag_name | cut -d '"' -f 4)
  if [ -z "$VERSION" ]; then
    echo "❌ Error: Could not fetch the latest version from GitHub."
    exit 1
  fi
  echo "✅ Latest version found: $VERSION."
else
  echo "✨ Using specified version: $VERSION."
fi

VERSION_NO_V="${VERSION#v}"
TARBALL="${BINARY_NAME}_${VERSION_NO_V}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL}"

# --- Create temporary directory for download and extraction ---
echo "📁 Creating temporary directory..."
TEMP_DIR=$(mktemp -d)
echo "✅ Created temporary directory: $TEMP_DIR"

# --- Download the tarball ---
echo "⬇️ Downloading $TARBALL from $URL..."
if ! curl -L "$URL" -o "$TEMP_DIR/$TARBALL"; then
    echo "❌ Error: Failed to download $TARBALL. Please check the version and repository."
    rm -rf "$TEMP_DIR" # Clean up temp dir
    exit 1
fi
echo "✅ Download complete."

# --- Extract the tarball ---
echo "📦 Extracting $TARBALL..."
if ! tar -xzf "$TEMP_DIR/$TARBALL" -C "$TEMP_DIR"; then
    echo "❌ Error: Failed to extract $TARBALL."
    rm -rf "$TEMP_DIR" # Clean up temp dir
    exit 1
fi
echo "✅ Extraction complete."

# --- Find the binary in the extracted files ---
EXTRACTED_BINARY="$TEMP_DIR/$BINARY_NAME"
if [ ! -f "$EXTRACTED_BINARY" ]; then
    # Sometimes the binary might be in a subdirectory named after the project/version
    # Let's try to find it
    FOUND_BINARY=$(find "$TEMP_DIR" -name "$BINARY_NAME" -type f -print -quit)
    if [ -f "$FOUND_BINARY" ]; then
        EXTRACTED_BINARY="$FOUND_BINARY"
        echo "💡 Found binary in subdirectory: $EXTRACTED_BINARY"
    else
        echo "❌ Error: Could not find the binary '$BINARY_NAME' in the extracted archive."
        ls "$TEMP_DIR" # List files in temp dir for debugging
        rm -rf "$TEMP_DIR" # Clean up temp dir
        exit 1
    fi
fi


# --- Determine installation path ---
INSTALL_PATH="/usr/local/bin"
FALLBACK_PATH="$HOME/.local/bin"
TARGET_BINARY_PATH="$INSTALL_PATH/$BINARY_NAME"

# Check if /usr/local/bin is writable or if sudo is available
if [ -w "$INSTALL_PATH" ] || command -v sudo >/dev/null; then
  echo "🚚 Attempting to install to $INSTALL_PATH..."
  # Check if binary already exists
  if [ -f "$TARGET_BINARY_PATH" ]; then
      read -r -p "⚠️ $BINARY_NAME already exists in $INSTALL_PATH. Overwrite? (y/N) " response
      case "$response" in
          [yY][eE][sS]|[yY])
              echo "Overwriting existing binary."
              if command -v sudo >/dev/null; then
                  sudo chmod +x "$EXTRACTED_BINARY"
                  sudo mv "$EXTRACTED_BINARY" "$TARGET_BINARY_PATH"
              else
                   chmod +x "$EXTRACTED_BINARY"
                   mv "$EXTRACTED_BINARY" "$TARGET_BINARY_PATH"
              fi
              ;;
          *)
              echo "Skipping installation to $INSTALL_PATH."
              # Clean up temp dir
              rm -rf "$TEMP_DIR"
              echo "✅ Installation process finished (skipped)."
              exit 0 # Exit successfully as user chose not to overwrite
              ;;
      esac
  else
      if command -v sudo >/dev/null; then
          sudo chmod +x "$EXTRACTED_BINARY"
          sudo mv "$EXTRACTED_BINARY" "$TARGET_BINARY_PATH"
      else
           chmod +x "$EXTRACTED_BINARY"
           mv "$EXTRACTED_BINARY" "$TARGET_BINARY_PATH"
      fi
  fi
else
  echo "⚠️ Insufficient permissions to install to $INSTALL_PATH and sudo not found."
  echo "🚚 Installing to $FALLBACK_PATH instead."
  mkdir -p "$FALLBACK_PATH"
  TARGET_BINARY_PATH="$FALLBACK_PATH/$BINARY_NAME"

  # Check if binary already exists in fallback path
  if [ -f "$TARGET_BINARY_PATH" ]; then
       read -r -p "⚠️ $BINARY_NAME already exists in $FALLBACK_PATH. Overwrite? (y/N) " response
      case "$response" in
          [yY][eE][sS]|[yY])
              echo "Overwriting existing binary."
              chmod +x "$EXTRACTED_BINARY"
              mv "$EXTRACTED_BINARY" "$TARGET_BINARY_PATH"
              ;;
          *)
              echo "Skipping installation to $FALLBACK_PATH."
              # Clean up temp dir
              rm -rf "$TEMP_DIR"
              echo "✅ Installation process finished (skipped)."
              exit 0 # Exit successfully as user chose not to overwrite
              ;;
      esac
  else
      chmod +x "$EXTRACTED_BINARY"
      mv "$EXTRACTED_BINARY" "$TARGET_BINARY_PATH"
  fi

  # Add fallback path to PATH if not already present
  PROFILE_FILES=("$HOME/.bashrc" "$HOME/.zshrc" "$HOME/.profile")
  PATH_SET=false
  for profile in "${PROFILE_FILES[@]}"; do
      if [ -f "$profile" ]; then
          if ! grep -q "$FALLBACK_PATH" "$profile"; then
              echo "Adding $FALLBACK_PATH to PATH in $profile"
              echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$profile"
              PATH_SET=true
          fi
      fi
  done

  if [ "$PATH_SET" = true ]; then
      echo "👉 Added '$FALLBACK_PATH' to your PATH. You may need to restart your shell or run 'source ~/.bashrc' (or your respective profile file)."
  else
       echo "💡 '$FALLBACK_PATH' is likely already in your PATH or no standard profile file found."
  fi
fi

# --- Clean up ---
echo "🧹 Cleaning up temporary directory $TEMP_DIR..."
rm -rf "$TEMP_DIR"
echo "✅ Cleanup complete."

echo "🎉 $BINARY_NAME $VERSION installed successfully to $TARGET_BINARY_PATH!"
