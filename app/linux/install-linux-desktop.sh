#!/usr/bin/env sh
# Install the release's launcher and icon for the current user only.
set -eu

app_id=io.github.daniele.ERMerchantEditor
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
data_home=${XDG_DATA_HOME:-"$HOME/.local/share"}
app_dir="$data_home/er-merchant-editor"
applications_dir="$data_home/applications"
icons_dir="$data_home/icons/hicolor/256x256/apps"

mkdir -p "$app_dir" "$applications_dir" "$icons_dir"
install -m 755 "$script_dir/ERMerchantEditor-linux-amd64" "$app_dir/ERMerchantEditor"
install -m 644 "$script_dir/$app_id.png" "$icons_dir/$app_id.png"
sed "s|@EXECUTABLE@|$app_dir/ERMerchantEditor|" \
  "$script_dir/$app_id.desktop" >"$applications_dir/$app_id.desktop"

command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database "$applications_dir" || true
command -v gtk-update-icon-cache >/dev/null 2>&1 && gtk-update-icon-cache -f "$data_home/icons/hicolor" || true

echo "Installed ER Merchant Editor for this user. Launch it from the app menu."
