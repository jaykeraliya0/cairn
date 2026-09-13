-- Machine-specific Hyprland settings.
--
-- The Cairn installer rewrites this file during installation with the keyboard
-- layout you chose and, on Nvidia hardware, the environment Wayland needs.
-- hyprland.lua requires it from its very last line, so anything set here
-- overrides the defaults above it.
--
-- What ships on the ISO is the fallback for a live session; edit it freely
-- once installed, the installer only ever writes it once.

hl.config({
    input = {
        kb_layout = "us",
    },
})
