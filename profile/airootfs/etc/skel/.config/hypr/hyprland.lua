-- Cairn default Hyprland config -- Last of Us theme.
--
-- Hyprland's own configuration language (hyprlang, the old .conf files) is
-- deprecated as of 0.55 and warns on every start; this is the Lua replacement.
-- See https://wiki.hypr.land/Configuring/Start/
--
-- Colors live in colors.lua, machine-specific settings in hardware.lua.

local colors = require("colors")

------------------
---- MONITORS ----
------------------

hl.monitor({
    output   = "",
    mode     = "preferred",
    position = "auto",
    scale    = 1,
})

---------------------
---- MY PROGRAMS ----
---------------------

local terminal    = "alacritty"
local fileManager = "nautilus"
local browser     = "firefox"
local menu        = "rofi -show drun"
local mainMod     = "SUPER"

-------------------
---- AUTOSTART ----
-------------------

hl.on("hyprland.start", function()
    hl.exec_cmd("waybar")
    hl.exec_cmd("mako")
    hl.exec_cmd("swaybg -i ~/.config/wallpaper.jpg -m fill")
    -- hyprpolkitagent ships only a systemd user unit, no binary in PATH.
    hl.exec_cmd("systemctl --user start hyprpolkitagent.service")
    hl.exec_cmd("hypridle")
    -- Feed the clipboard history that SUPER+V reads back.
    hl.exec_cmd("wl-paste --type text --watch ~/.config/hypr/scripts/clipboard.sh store")
    hl.exec_cmd("wl-paste --type image --watch ~/.config/hypr/scripts/clipboard.sh store")
end)

-------------------------------
---- ENVIRONMENT VARIABLES ----
-------------------------------

hl.env("XCURSOR_SIZE", "24")
hl.env("HYPRCURSOR_SIZE", "24")

-- Make Qt applications follow qt6ct instead of defaulting to a light Fusion
-- theme next to every dark GTK window. GTK's own dark preference lives in
-- ~/.config/gtk-3.0/settings.ini and gtk-4.0/settings.ini.
hl.env("QT_QPA_PLATFORMTHEME", "qt6ct")

-----------------------
---- LOOK AND FEEL ----
-----------------------

hl.config({
    general = {
        gaps_in     = 2,
        gaps_out    = 4,
        border_size = 1,

        col = {
            active_border   = colors.active_border,
            inactive_border = colors.inactive_border,
        },

        resize_on_border = false,
        allow_tearing    = false,
        layout           = "dwindle",
    },

    decoration = {
        rounding       = 3,
        rounding_power = 2,

        shadow = {
            enabled      = true,
            range        = 14,
            render_power = 2,
            color        = colors.shadow,
        },

        blur = {
            enabled  = true,
            size     = 4,
            passes   = 3,
            vibrancy = 0,
            xray     = true,
        },
    },

    animations = {
        enabled = true,
    },

    dwindle = {
        preserve_split = true,
    },

    misc = {
        force_default_wallpaper = 0,
        disable_hyprland_logo   = true,
        -- Safety: make sure a blanked screen can always be woken by input.
        key_press_enables_dpms  = true,
        mouse_move_enables_dpms = true,
    },

    input = {
        -- kb_layout lives in hardware.lua, which the installer writes from the
        -- keyboard layout chosen during installation. It is required at the
        -- end of this file, so it wins over anything set here.
        follow_mouse = 1,
        sensitivity  = 0,

        touchpad = {
            natural_scroll = true,
        },
    },
})

----------------------
---- ANIMATIONS ------
----------------------

hl.curve("easeOutQuint",   { type = "bezier", points = { { 0.23, 1 },    { 0.32, 1 } } })
hl.curve("easeInOutCubic", { type = "bezier", points = { { 0.65, 0.05 }, { 0.36, 1 } } })
hl.curve("linear",         { type = "bezier", points = { { 0, 0 },       { 1, 1 } } })
hl.curve("almostLinear",   { type = "bezier", points = { { 0.5, 0.5 },   { 0.75, 1 } } })
hl.curve("quick",          { type = "bezier", points = { { 0.15, 0 },    { 0.1, 1 } } })

hl.animation({ leaf = "global",        enabled = true, speed = 10,  bezier = "default" })
hl.animation({ leaf = "border",        enabled = true, speed = 5.4, bezier = "easeOutQuint" })
hl.animation({ leaf = "windows",       enabled = true, speed = 3,   bezier = "easeOutQuint", style = "slide" })
hl.animation({ leaf = "windowsIn",     enabled = true, speed = 2.5, bezier = "easeOutQuint", style = "popin 90%" })
hl.animation({ leaf = "windowsOut",    enabled = true, speed = 2,   bezier = "easeOutQuint", style = "popin 90%" })
hl.animation({ leaf = "fadeIn",        enabled = true, speed = 1.7, bezier = "almostLinear" })
hl.animation({ leaf = "fadeOut",       enabled = true, speed = 1.5, bezier = "almostLinear" })
hl.animation({ leaf = "fade",          enabled = true, speed = 3,   bezier = "quick" })
hl.animation({ leaf = "layers",        enabled = true, speed = 3.8, bezier = "easeOutQuint" })
hl.animation({ leaf = "layersIn",      enabled = true, speed = 4,   bezier = "easeOutQuint", style = "fade" })
hl.animation({ leaf = "layersOut",     enabled = true, speed = 1.5, bezier = "linear",       style = "fade" })
hl.animation({ leaf = "workspaces",    enabled = true, speed = 1.9, bezier = "almostLinear", style = "fade" })
hl.animation({ leaf = "workspacesIn",  enabled = true, speed = 1.2, bezier = "almostLinear", style = "fade" })
hl.animation({ leaf = "workspacesOut", enabled = true, speed = 1.9, bezier = "almostLinear", style = "fade" })

--------------------------------
---- WINDOWS AND WORKSPACES ----
--------------------------------

hl.layer_rule({
    name    = "rofi-no-anim",
    match   = { namespace = "^rofi$" },
    no_anim = true,
})

hl.layer_rule({
    name         = "notifications-blur",
    match        = { namespace = "^notifications$" },
    blur         = true,
    ignore_alpha = 0.2,
})

hl.window_rule({
    name   = "float-term",
    match  = { title = "^(float-term)$" },
    float  = true,
    size   = "1000 600",
    center = true,
})

---------------------
---- KEYBINDINGS ----
---------------------

hl.bind(mainMod .. " + Return",         hl.dsp.exec_cmd(terminal))
hl.bind(mainMod .. " + SHIFT + Return", hl.dsp.exec_cmd(terminal .. " --title float-term"))
hl.bind(mainMod .. " + SPACE",          hl.dsp.exec_cmd(menu))

hl.bind(mainMod .. " + Q", hl.dsp.window.close())
hl.bind(mainMod .. " + T", hl.dsp.window.float({ action = "toggle" }))
hl.bind(mainMod .. " + F", hl.dsp.window.fullscreen())
hl.bind(mainMod .. " + P", hl.dsp.window.pseudo())
hl.bind(mainMod .. " + J", hl.dsp.layout("togglesplit"))
hl.bind(mainMod .. " + E",         hl.dsp.exec_cmd(fileManager))
hl.bind(mainMod .. " + SHIFT + F", hl.dsp.exec_cmd(fileManager))
hl.bind(mainMod .. " + SHIFT + B", hl.dsp.exec_cmd(browser))
hl.bind(mainMod .. " + L",         hl.dsp.exec_cmd("hyprlock"))
hl.bind(mainMod .. " + N",         hl.dsp.exec_cmd("makoctl dismiss -a"))
hl.bind(mainMod .. " + V",         hl.dsp.exec_cmd("~/.config/hypr/scripts/clipboard.sh menu"))
hl.bind(mainMod .. " + SHIFT + V", hl.dsp.exec_cmd("pavucontrol"))

-- Focus movement, and moving the window, with the arrow keys.
for key, direction in pairs({ left = "left", right = "right", up = "up", down = "down" }) do
    hl.bind(mainMod .. " + " .. key,           hl.dsp.focus({ direction = direction }))
    hl.bind(mainMod .. " + SHIFT + " .. key,   hl.dsp.window.move({ direction = direction }))
end

-- Workspaces 1-10, with 10 on the 0 key.
for i = 1, 10 do
    local key = i % 10
    hl.bind(mainMod .. " + " .. key,         hl.dsp.focus({ workspace = i }))
    hl.bind(mainMod .. " + SHIFT + " .. key, hl.dsp.window.move({ workspace = i }))
end

hl.bind(mainMod .. " + mouse_down", hl.dsp.focus({ workspace = "e+1" }))
hl.bind(mainMod .. " + mouse_up",   hl.dsp.focus({ workspace = "e-1" }))

hl.bind(mainMod .. " + mouse:272", hl.dsp.window.drag(),   { mouse = true })
hl.bind(mainMod .. " + mouse:273", hl.dsp.window.resize(), { mouse = true })

-- Screenshots: region -> save + copy, full screen -> save + copy.
hl.bind("Print",                  hl.dsp.exec_cmd("~/.config/hypr/scripts/screenshot.sh region"))
hl.bind(mainMod .. " + Print",    hl.dsp.exec_cmd("~/.config/hypr/scripts/screenshot.sh screen"))
hl.bind(mainMod .. " + SHIFT + Print", hl.dsp.exec_cmd("~/.config/hypr/scripts/screenshot.sh copy"))

-- Media, volume and brightness. Locked so they work on the lock screen.
hl.bind("XF86AudioRaiseVolume",  hl.dsp.exec_cmd("wpctl set-volume -l 1 @DEFAULT_AUDIO_SINK@ 5%+"), { locked = true, repeating = true })
hl.bind("XF86AudioLowerVolume",  hl.dsp.exec_cmd("wpctl set-volume @DEFAULT_AUDIO_SINK@ 5%-"),      { locked = true, repeating = true })
hl.bind("XF86AudioMute",         hl.dsp.exec_cmd("wpctl set-mute @DEFAULT_AUDIO_SINK@ toggle"),     { locked = true })
hl.bind("XF86MonBrightnessUp",   hl.dsp.exec_cmd("brightnessctl -e4 -n2 set 5%+"),                  { locked = true, repeating = true })
hl.bind("XF86MonBrightnessDown", hl.dsp.exec_cmd("brightnessctl -e4 -n2 set 5%-"),                  { locked = true, repeating = true })

------------------------------
---- MACHINE-SPECIFIC ------- -
------------------------------

-- Written by the Cairn installer: the keyboard layout, and on Nvidia hardware
-- the environment Wayland needs. Required last so it overrides the defaults.
-- pcall so a session still starts if someone deletes the file.
pcall(require, "hardware")
