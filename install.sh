#!/bin/bash

# Configuration
APP_NAME="govfs"
CLI_NAME="govfs-cli"
GITHUB_REPO="${GITHUB_REPO:-meteormin/govfs}"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')"
BIN_NAME="govfs-${OS}-${ARCH}"
CLI_BIN_NAME="govfs-cli-${OS}-${ARCH}"

INSTALL_PATH="/usr/local/bin/$APP_NAME"
CLI_INSTALL_PATH="/usr/local/bin/$CLI_NAME"

# App Workspace
APP_DIR="$HOME/.$APP_NAME"
CONFIG_PATH="$APP_DIR/config.toml"
BIN_URL="https://github.com/${GITHUB_REPO}/releases/latest/download/${BIN_NAME}"
CLI_BIN_URL="https://github.com/${GITHUB_REPO}/releases/latest/download/${CLI_BIN_NAME}"
CONFIG_URL="https://github.com/${GITHUB_REPO}/releases/latest/download/config.toml"

echo "🚀 Starting installation for ${GITHUB_REPO}..."
echo "📍 Workspace: $APP_DIR"

echo "📁 Setup workspace directory..."
mkdir -p "$APP_DIR"

echo "📥 Downloading config.toml from latest GitHub release..."
if command -v curl >/dev/null 2>&1; then
    curl -sfL "$CONFIG_URL" -o "$CONFIG_PATH" || { echo "❌ Error: Failed to download config.toml."; exit 1; }
else
    wget -qO "$CONFIG_PATH" "$CONFIG_URL" || { echo "❌ Error: Failed to download config.toml."; exit 1; }
fi

# 2. 서버 및 CLI 바이너리 다운로드 및 설치
cleanup() {
    rm -f "$TMP_BIN" "$TMP_CLI_BIN"
}
trap cleanup EXIT

echo "📥 Downloading server binary ($BIN_NAME) from latest GitHub release..."
TMP_BIN="/tmp/$BIN_NAME"
if command -v curl >/dev/null 2>&1; then
    curl -sfL "$BIN_URL" -o "$TMP_BIN" || { echo "❌ Error: Failed to download binary $BIN_NAME."; exit 1; }
else
    wget -qO "$TMP_BIN" "$BIN_URL" || { echo "❌ Error: Failed to download binary $BIN_NAME."; exit 1; }
fi

echo "📥 Downloading CLI binary ($CLI_BIN_NAME) from latest GitHub release..."
TMP_CLI_BIN="/tmp/$CLI_BIN_NAME"
if command -v curl >/dev/null 2>&1; then
    curl -sfL "$CLI_BIN_URL" -o "$TMP_CLI_BIN" || { echo "❌ Error: Failed to download CLI binary $CLI_BIN_NAME."; exit 1; }
else
    wget -qO "$TMP_CLI_BIN" "$CLI_BIN_URL" || { echo "❌ Error: Failed to download CLI binary $CLI_BIN_NAME."; exit 1; }
fi

echo "⚙️  Installing binaries to /usr/local/bin (requires sudo)..."
sudo mv "$TMP_BIN" "$INSTALL_PATH"
sudo chmod +x "$INSTALL_PATH"

sudo mv "$TMP_CLI_BIN" "$CLI_INSTALL_PATH"
sudo chmod +x "$CLI_INSTALL_PATH"

# 3. OS별 서비스 등록 분기
OS_TYPE="$(uname)"

if [ "$OS_TYPE" == "Darwin" ]; then
    # macOS: launchd 사용
    PLIST_PATH="$HOME/Library/LaunchAgents/com.$APP_NAME.plist"
    echo "Registering macOS launchd service..."
    cat <<EOF > "$PLIST_PATH"
<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
    <key>Label</key><string>com.$APP_NAME</string>
    <key>WorkingDirectory</key><string>$APP_DIR</string>
    <key>ProgramArguments</key>
    <array>
        <string>$INSTALL_PATH</string>
        <string>-config</string>
        <string>$CONFIG_PATH</string>
    </array>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><true/>
</dict>
</plist>
EOF
    launchctl unload "$PLIST_PATH" 2>/dev/null || true
    launchctl load "$PLIST_PATH"

elif [ "$OS_TYPE" == "Linux" ]; then
    # Linux: systemd 사용 (User mode)
    SYSTEMD_DIR="$HOME/.config/systemd/user"
    mkdir -p "$SYSTEMD_DIR"
    SERVICE_PATH="$SYSTEMD_DIR/$APP_NAME.service"
    
    echo "Registering Linux systemd user service..."
    cat <<EOF > "$SERVICE_PATH"
[Unit]
Description=$APP_NAME Daemon
[Service]
WorkingDirectory=$APP_DIR
ExecStart=$INSTALL_PATH -config $CONFIG_PATH
Restart=always
[Install]
WantedBy=default.target
EOF
    systemctl --user daemon-reload
    systemctl --user enable "$APP_NAME"
    systemctl --user start "$APP_NAME"
fi

echo "Installation successful on $OS_TYPE."