package alerts

import (
	"database/sql"
	"errors"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// UpsertAlerts handles global alert creation and updates across systems.
func UpsertAlerts(e *core.RequestEvent) error {
	reqData := struct {
		Min       uint8    `json:"min"`
		Value     float64  `json:"value"`
		Name      string   `json:"name"`
		Systems   []string `json:"systems"`
		Overwrite bool     `json:"overwrite"`
	}{}
	err := e.BindBody(&reqData)
	if err != nil || reqData.Name == "" || len(reqData.Systems) == 0 {
		return e.BadRequestError("Bad data", err)
	}

	alertsCollection, err := e.App.FindCachedCollectionByNameOrId("alerts")
	if err != nil {
		return err
	}

	err = e.App.RunInTransaction(func(txApp core.App) error {
		for _, systemId := range reqData.Systems {
			// find existing matching alert
			alertRecord, err := txApp.FindFirstRecordByFilter(alertsCollection,
				"system={:system} && name={:name}",
				dbx.Params{"system": systemId, "name": reqData.Name})

			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}

			// skip if alert already exists and overwrite is not set
			if !reqData.Overwrite && alertRecord != nil {
				continue
			}

			// create new alert if it doesn't exist
			if alertRecord == nil {
				alertRecord = core.NewRecord(alertsCollection)
				alertRecord.Set("system", systemId)
				alertRecord.Set("name", reqData.Name)
			}

			alertRecord.Set("value", reqData.Value)
			alertRecord.Set("min", reqData.Min)

			if err := txApp.SaveNoValidate(alertRecord); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	return e.JSON(http.StatusOK, map[string]any{"success": true})
}

// DeleteAlerts handles global alert deletion across systems.
func DeleteAlerts(e *core.RequestEvent) error {
	reqData := struct {
		AlertName string   `json:"name"`
		Systems   []string `json:"systems"`
	}{}
	err := e.BindBody(&reqData)
	if err != nil || reqData.AlertName == "" || len(reqData.Systems) == 0 {
		return e.BadRequestError("Bad data", err)
	}

	var numDeleted uint16

	err = e.App.RunInTransaction(func(txApp core.App) error {
		for _, systemId := range reqData.Systems {
			// Find existing alert to delete
			alertRecord, err := txApp.FindFirstRecordByFilter("alerts",
				"system={:system} && name={:name}",
				dbx.Params{"system": systemId, "name": reqData.AlertName})

			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					// alert doesn't exist, continue to next system
					continue
				}
				return err
			}

			if err := txApp.Delete(alertRecord); err != nil {
				return err
			}
			numDeleted++
		}
		return nil
	})

	if err != nil {
		return err
	}

	return e.JSON(http.StatusOK, map[string]any{"success": true, "count": numDeleted})
}

// SendTestNotification sends a test message using the saved Telegram settings.
func (am *AlertManager) SendTestNotification(e *core.RequestEvent) error {
	record, err := e.App.FindFirstRecordByFilter("notification_settings", "id='globalsettings1'")
	if err != nil {
		return e.InternalServerError("Notification settings are unavailable", err)
	}
	var settings NotificationSettings
	if err := record.UnmarshalJSONField("settings", &settings); err != nil || settings.TelegramBotToken == "" || len(settings.TelegramUserIDs) == 0 {
		return e.BadRequestError("Telegram bot token and allowed user IDs are required", err)
	}
	for _, userID := range settings.TelegramUserIDs {
		if err = am.SendTelegramAlert(settings.TelegramBotToken, userID, "Test Alert", "This is a notification from Picket.", am.hub.MakeLink()); err != nil {
			return e.InternalServerError("Telegram test failed: "+err.Error(), err)
		}
	}
	return e.JSON(http.StatusOK, map[string]bool{"sent": true})
}

// isInternalURL checks if the given shoutrrr URL points to an internal destination (localhost or private IP)
func isInternalURL(rawURL string) (bool, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false, err
	}

	host := parsedURL.Hostname()
	if host == "" {
		return false, nil
	}

	if strings.EqualFold(host, "localhost") {
		return true, nil
	}

	if ip := net.ParseIP(host); ip != nil {
		return isInternalIP(ip), nil
	}

	// Some Shoutrrr URLs use the host position for service identifiers rather than a
	// network hostname (for example, discord://token@webhookid). Restrict DNS lookups
	// to names that look like actual hostnames so valid service URLs keep working.
	if !strings.Contains(host, ".") {
		return false, nil
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return false, nil
	}

	if slices.ContainsFunc(ips, isInternalIP) {
		return true, nil
	}

	return false, nil
}

func isInternalIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified()
}
