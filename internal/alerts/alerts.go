// Package alerts handles alert management and delivery.
package alerts

import (
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/nicholas-fedor/shoutrrr"
	"github.com/pocketbase/pocketbase/core"
)

type hubLike interface {
	core.App
	MakeLink(parts ...string) string
}

type AlertManager struct {
	hub           hubLike
	stopOnce      sync.Once
	pendingAlerts sync.Map
	alertsCache   *AlertsCache
}

type AlertMessageData struct {
	SystemID string
	Title    string
	Message  string
	Link     string
	LinkText string
}

type NotificationSettings struct {
	TelegramBotToken string   `json:"telegramBotToken"`
	TelegramUserIDs  []string `json:"telegramUserIds"`
}

type SystemAlertFsStats struct {
	DiskTotal float64 `json:"d"`
	DiskUsed  float64 `json:"du"`
}

// Values pulled from system_stats.stats that are relevant to alerts.
type SystemAlertStats struct {
	Cpu       float64                       `json:"cpu"`
	Mem       float64                       `json:"mp"`
	Disk      float64                       `json:"dp"`
	Bandwidth [2]uint64                     `json:"b"`
	GPU       map[string]SystemAlertGPUData `json:"g"`
	LoadAvg   [3]float64                    `json:"la"`
	ExtraFs   map[string]SystemAlertFsStats `json:"efs"`
}

type SystemAlertGPUData struct {
	Usage float64 `json:"u"`
}

type SystemAlertData struct {
	systemRecord *core.Record
	alertData    CachedAlertData
	name         string
	unit         string
	val          float64
	threshold    float64
	triggered    bool
	time         time.Time
	count        uint8
	min          uint8
	mapSums      map[string]float32
	descriptor   string // override descriptor in notification body (for temp sensor, disk partition, etc)
}

// notification services that support title param
var supportsTitle = map[string]struct{}{
	"bark":       {},
	"discord":    {},
	"gotify":     {},
	"ifttt":      {},
	"join":       {},
	"lark":       {},
	"ntfy":       {},
	"opsgenie":   {},
	"pushbullet": {},
	"pushover":   {},
	"slack":      {},
	"teams":      {},
	"telegram":   {},
	"zulip":      {},
}

// NewAlertManager creates a new AlertManager instance.
func NewAlertManager(app hubLike) *AlertManager {
	am := &AlertManager{
		hub:         app,
		alertsCache: NewAlertsCache(app),
	}
	am.bindEvents()
	return am
}

// Bind events to the alerts collection lifecycle
func (am *AlertManager) bindEvents() {
	am.hub.OnRecordAfterUpdateSuccess("alerts").BindFunc(updateHistoryOnAlertUpdate)
	am.hub.OnRecordAfterDeleteSuccess("alerts").BindFunc(resolveHistoryOnAlertDelete)
	am.hub.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// Populate all alerts into cache on startup
		_ = am.alertsCache.PopulateFromDB(true)

		if err := resolveStatusAlerts(e.App); err != nil {
			e.App.Logger().Error("Failed to resolve stale status alerts", "err", err)
		}
		if err := am.restorePendingStatusAlerts(); err != nil {
			e.App.Logger().Error("Failed to restore pending status alerts", "err", err)
		}
		return e.Next()
	})
}

// SendAlert sends an alert using the singleton hub-managed settings.
func (am *AlertManager) SendAlert(data AlertMessageData) error {
	record, err := am.hub.FindFirstRecordByFilter("notification_settings", "id='globalsettings1'")
	if err != nil {
		return err
	}
	notificationSettings := NotificationSettings{
		TelegramBotToken: "",
		TelegramUserIDs:  []string{},
	}
	if err := record.UnmarshalJSONField("settings", &notificationSettings); err != nil {
		am.hub.Logger().Error("Failed to unmarshal user settings", "err", err)
	}
	notificationSettings.TelegramUserIDs = NonEmptyTelegramUserIDs(notificationSettings.TelegramUserIDs)
	if notificationSettings.TelegramBotToken == "" || len(notificationSettings.TelegramUserIDs) == 0 {
		return nil
	}
	for _, userID := range notificationSettings.TelegramUserIDs {
		if err := am.SendTelegramAlert(notificationSettings.TelegramBotToken, userID, data.Title, data.Message, data.Link); err != nil {
			return err
		}
	}
	return nil
}

func (am *AlertManager) SendTelegramAlert(token, chatID, title, message, link string) error {
	telegramURL := fmt.Sprintf("telegram://%s@telegram?channels=%s", url.PathEscape(token), url.QueryEscape(chatID))
	return am.SendShoutrrrAlert(telegramURL, title, message, link, "View Picket")
}

// SendShoutrrrAlert sends an alert via a Shoutrrr URL
func (am *AlertManager) SendShoutrrrAlert(notificationUrl, title, message, link, linkText string) error {
	// Parse the URL
	parsedURL, err := url.Parse(notificationUrl)
	if err != nil {
		return fmt.Errorf("error parsing URL: %v", err)
	}
	scheme := parsedURL.Scheme
	queryParams := parsedURL.Query()

	// Add title
	if _, ok := supportsTitle[scheme]; ok {
		queryParams.Add("title", title)
	} else if scheme == "mattermost" {
		// use markdown title for mattermost
		message = "##### " + title + "\n\n" + message
	} else if scheme == "generic" && queryParams.Has("template") {
		// add title as property if using generic with template json
		titleKey := queryParams.Get("titlekey")
		if titleKey == "" {
			titleKey = "title"
		}
		queryParams.Add("$"+titleKey, title)
	} else {
		// otherwise just add title to message
		message = title + "\n\n" + message
	}

	// Add a link only when the caller supplied one. Telegram test messages are
	// intentionally plain and must not include the dashboard URL.
	if link != "" {
		switch scheme {
		case "ntfy":
			queryParams.Add("Actions", fmt.Sprintf("view, %s, %s", linkText, link))
		case "lark":
			queryParams.Add("link", link)
		case "bark":
			queryParams.Add("url", link)
		default:
			message += "\n\n" + link
		}
	}

	// Encode the modified query parameters back into the URL
	parsedURL.RawQuery = queryParams.Encode()
	// log.Println("URL after modification:", parsedURL.String())

	err = shoutrrr.Send(parsedURL.String(), message)

	if err == nil {
		am.hub.Logger().Info("Sent shoutrrr alert", "title", title)
	} else {
		am.hub.Logger().Error("Error sending shoutrrr alert", "err", err)
		return err
	}
	return nil
}

// setAlertTriggered updates the "triggered" status of an alert record in the database
func (am *AlertManager) setAlertTriggered(alert CachedAlertData, triggered bool) error {
	alertRecord, err := am.hub.FindRecordById("alerts", alert.Id)
	if err != nil {
		return err
	}
	alertRecord.Set("triggered", triggered)
	return am.hub.Save(alertRecord)
}
