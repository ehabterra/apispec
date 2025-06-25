package core

// --- Event System (Observer Pattern) ---
type ParseEvent string

const (
	EventRouteFound ParseEvent = "route_found"
)

// EventListener defines the interface for parser events
type EventListener interface {
	OnRouteParsed(route *ParsedRoute)
}
