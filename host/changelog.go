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
