package main

type releaseNote struct {
	Version string   `json:"version"`
	Status  string   `json:"status"`
	Changes []string `json:"changes"`
}

var releaseNotes = []releaseNote{
	{
		Version: "v0.1.15 Beta",
		Status:  "Unpublished candidate",
		Changes: []string{
			"Combines the desktop dashboard, expanded weather descriptions, song-text fallback, optional Today agenda and notification controls with the v0.1.14 audio/time-sync base.",
			"Adds the shared changelog, native dashboard resizing and TomTom face/layout/theme presets.",
			"Windows visual, Store and physical-device acceptance remain outstanding.",
		},
	},
	{
		Version: "v0.1.14",
		Status:  "Latest published prerelease",
		Changes: []string{
			"Added opt-in Linux/Windows system-audio forwarding with adjustable TomTom output gain.",
			"Added host-to-device time synchronization and compact Quick settings.",
		},
	},
	{
		Version: "v0.1.13 Beta",
		Status:  "In progress; unpublished",
		Changes: []string{
			"Native dashboard adjusts to resized windows; Windows visual verification is pending.",
		},
	},
	{
		Version: "v0.1.12 Beta",
		Status:  "Unpublished candidate",
		Changes: []string{
			"Reorganized the web dashboard with responsive sections and local SVG icons.",
			"Expanded weather labels for clouds, precipitation, fog, snow and storms.",
			"Improved song-title fallback for text unsupported by the device font.",
			"Consolidated the Windows Start menu to one FractureOS dashboard shortcut.",
			"Added a disabled-by-default Today agenda with bounded recurring calendar summaries.",
			"Added notification quiet hours, app allow/block filters and delivery diagnostics.",
		},
	},
	{
		Version: "v0.1.11",
		Status:  "Latest published stable release",
		Changes: []string{
			"Added timed lyrics, grouped Clock tools and Quick settings.",
			"Added a locally rendered Windows home preview and adaptive information dock.",
		},
	},
}
