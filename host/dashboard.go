package main

func weatherDescription(code int) string {
	switch {
	case code >= 95:
		return "Thunderstorm"
	case code == 66 || code == 67:
		return "Freezing rain"
	case code >= 71 && code <= 77 || code == 85 || code == 86:
		return "Snow"
	case code >= 80:
		return "Rain showers"
	case code >= 61:
		return "Rain"
	case code >= 51:
		return "Drizzle"
	case code >= 45:
		return "Fog"
	case code >= 2:
		return "Cloudy"
	default:
		return "Clear"
	}
}
