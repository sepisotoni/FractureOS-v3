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
		Version: "v0.1.15 Beta",
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
