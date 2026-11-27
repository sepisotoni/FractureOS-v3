package main

// releaseNotes is compiled into the app, so the notes always match the installed
// version and arrive with every update. Nothing here is fetched over the network.
type releaseNote struct {
	Version string   `json:"version"`
	Status  string   `json:"status"`
	Changes []string `json:"changes"`
}

var releaseNotes = []releaseNote{
	{
		Version: "v0.1.18.1",
		Status:  "Prerelease",
		Changes: []string{
			"Wallpaper now lives under Appearance (Clock & faces | Wallpaper). Choosing a wallpaper only previews it; nothing is sent to the TomTom until you press Apply to TomTom.",
			"Restore previous returns to your previous wallpaper selection. FractureOS does not back up the wallpaper file stored on the TomTom itself.",
			"A wallpaper chosen while the TomTom is unplugged, or lost if the TomTom is re-flashed, is applied automatically when it connects.",
			"A solid colour records the theme and face background it replaces and puts exactly those back when you pick something else; Legacy black changes no theme or face colours.",
			"Windows Setup waits for FractureOS processes in the selected install folder to close before replacing files.",
			"The native dashboard resizes and starts the tray when you use Direct USB.",
			"Choose a city or town for weather. The web Overview can capture the current TomTom screen on request.",
			"Control the hour-only clock display and menu timeout from the Windows dashboard.",
			"Fixed: \"hub: 404 page not found\" when the app is newer than the Linux hub; the message now says to update the hub or use Direct USB.",
			"Fixed: opening the dashboard on a specific page (?page=) showed an error.",
			"Updates now accept four-part versions such as 0.1.18.1.",
		},
	},
	{
		Version: "v0.1.18",
		Status:  "Prerelease",
		Changes: []string{
			"Added automatic PC wallpaper forwarding on Windows, solid-color legacy background option, TomTom ONE v6 preset pack, and a custom crop/zoom/pan studio with last-5 history.",
			"Added escalating frosted glassmorphism and background blur across TomTom menus so text and icons stay readable over wallpapers.",
			"Added top-right HH:MM clock across TomTom menus and Today agenda.",
			"Expanded media title ASCII transliteration with Cyrillic, Greek, fullwidth ASCII, and common symbol fallbacks.",
		},
	},
	{
		Version: "v0.1.17",
		Status:  "Release",
		Changes: []string{
			"Added Direct USB PC mode so Windows can host the TomTom directly without a separate Linux hub, alongside Remote Linux hub mode.",
			"Added native Windows CPU and RAM collection and relaxed device audio subnet validation for USB-connected PCs.",
			"Redesigned the web dashboard with dark frosted glassmorphism and an interactive 320x240 Home Preview.",
			"Reorganized the native Windows dashboard cards and consolidated Windows Start menu shortcuts to a single FractureOS entry.",
			"Fixed duplicate /api/audio route registration, duplicate audio-start dispatch, and framebuffer file descriptor leaks.",
		},
	},
	{
		Version: "v0.1.15",
		Status:  "Prerelease",
		Changes: []string{
			"Desktop dashboard rework with responsive sections and local icons.",
			"Expanded weather descriptions and a song-title fallback for text the device font cannot show.",
			"Optional Today agenda and notification controls (quiet hours, app allow/block lists, delivery diagnostics).",
			"Native dashboard resizing, TomTom face/layout/theme presets and this What's new view.",
		},
	},
	{
		Version: "v0.1.14",
		Status:  "Prerelease",
		Changes: []string{
			"Added opt-in Linux/Windows system-audio forwarding with adjustable TomTom output gain.",
			"Added host-to-device time synchronization and compact Quick settings.",
		},
	},
	{
		Version: "v0.1.11",
		Status:  "Stable",
		Changes: []string{
			"Added timed lyrics, grouped Clock tools and Quick settings.",
			"Added a locally rendered Windows home preview and adaptive information dock.",
		},
	},
}
