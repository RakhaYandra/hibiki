package event

// Journal PRIORITY is the syslog level, so notice and warning share a bucket
// and the three highest levels share critical (PRD 7.4).
func SeverityFromPriority(raw string) Severity {
	switch raw {
	case "0", "1", "2":
		return SeverityCritical
	case "3":
		return SeverityError
	case "4", "5":
		return SeverityWarning
	case "6":
		return SeverityInfo
	case "7":
		return SeverityDebug
	default:
		return SeverityUnknown
	}
}
