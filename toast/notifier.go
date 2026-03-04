// Package toast defines the Notifier interface for showing toast notifications.
package toast

// Notifier allows showing toast notifications decoupled from the window implementation.
type Notifier interface {
	ShowToast(message string)
	ShowErrorToast(message string)
}
