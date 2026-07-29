package hub

import "github.com/pocketbase/pocketbase/core"

var applicationCollections = map[string]struct{}{
	"systems":               {},
	"system_stats":          {},
	"containers":            {},
	"container_stats":       {},
	"system_details":        {},
	"systemd_services":      {},
	"alerts":                {},
	"alerts_history":        {},
	"notification_settings": {},
}

// setCollectionAccessSettings keeps the public application surface separate
// from PocketBase's administrative APIs.
func setCollectionAccessSettings(app core.App) error {
	for name := range applicationCollections {
		collection, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return err
		}
		if err := app.Save(collection); err != nil {
			return err
		}
	}
	return nil
}
