import type { ClientResponseError } from "pocketbase"
import { useStore } from "@nanostores/react"
import { BellIcon, LoaderCircleIcon, SaveIcon } from "lucide-react"
import { useEffect, useState } from "react"
import * as v from "valibot"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { InputTags } from "@/components/ui/input-tags"
import { Label } from "@/components/ui/label"
import { toast } from "@/components/ui/use-toast"
import { pb } from "@/lib/api"
import { alertInfo } from "@/lib/alerts"
import { $alerts, $systems } from "@/lib/stores"
import type { NotificationSettings } from "@/types"
import { AlertContent } from "@/components/alerts/alerts-sheet"

const schema = v.object({ telegramBotToken: v.string(), telegramUserIds: v.array(v.string()) })
const endpoint = "/api/picket/notification-settings"

export default function Notifications() {
	const [settings, setSettings] = useState<NotificationSettings>({ telegramBotToken: "", telegramUserIds: [] })
	const [loading, setLoading] = useState(true)
	const [overwriteExisting, setOverwriteExisting] = useState(false)
	const systems = useStore($systems)
	const alerts = useStore($alerts)

	useEffect(() => {
		pb.send<Partial<NotificationSettings>>(endpoint, {}).then((next) => setSettings({
			telegramBotToken: next.telegramBotToken ?? "",
			telegramUserIds: Array.isArray(next.telegramUserIds) ? next.telegramUserIds : [],
		})).catch((error) => {
			console.error(error); toast({ title: "Unable to load notification settings", variant: "destructive" })
		}).finally(() => setLoading(false))
	}, [])

	async function save() {
		setLoading(true)
		try {
			const parsed = v.parse(schema, settings)
			setSettings(await pb.send<NotificationSettings>(endpoint, { method: "PUT", body: parsed }))
			toast({ title: "Telegram notification settings saved" })
		} catch (error) {
			toast({ title: "Unable to save notification settings", description: (error as Error).message, variant: "destructive" })
		} finally { setLoading(false) }
	}

	return (
		<div className="space-y-5">
			<p className="text-sm text-muted-foreground"><BellIcon className="inline size-4 me-1" />Configure Telegram delivery for alerts created from the bell buttons in the systems table. Leave both fields blank to disable Telegram.</p>
			<div className="grid gap-2"><Label htmlFor="telegram-bot-token">Telegram bot token</Label><Input id="telegram-bot-token" type="password" value={settings.telegramBotToken} onChange={(event) => setSettings({ ...settings, telegramBotToken: event.target.value })} placeholder="123456:ABC..." /></div>
			<div className="grid gap-2"><Label htmlFor="telegram-user-ids">Allowed Telegram user IDs</Label><InputTags id="telegram-user-ids" value={settings.telegramUserIds} onChange={(telegramUserIds) => setSettings({ ...settings, telegramUserIds })} placeholder="Enter a user ID..." /><p className="text-xs text-muted-foreground">Press Enter or comma after each ID. Alerts are sent only to these users.</p></div>
			<div className="flex flex-wrap gap-2"><Button className="gap-2" onClick={save} disabled={loading}>{loading ? <LoaderCircleIcon className="size-4 animate-spin" /> : <SaveIcon className="size-4" />}<span>Save Telegram Settings</span></Button><Button variant="outline" onClick={test} disabled={loading || !settings.telegramBotToken || !settings.telegramUserIds.length}>Send Test</Button></div>
			<div className="border-t pt-6 space-y-4">
				<div><h3 className="text-lg font-medium">Global system alerts</h3><p className="text-sm text-muted-foreground">Configure one alert policy for all connected systems.</p></div>
				{systems.length === 0 ? <p className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">No systems connected.</p> : <>
					<label htmlFor="global-alert-overwrite" className="flex cursor-pointer items-center gap-2 rounded-md border border-destructive px-4 py-3 text-sm font-semibold text-destructive">
						<Checkbox id="global-alert-overwrite" checked={overwriteExisting} onCheckedChange={(checked) => setOverwriteExisting(checked === true)} className="text-destructive border-destructive data-[state=checked]:bg-destructive" />
						Overwrite existing system alert settings
					</label>
					<div className="grid gap-3">
						{Object.keys(alertInfo).map((name) => <AlertContent key={name} alertKey={name} data={alertInfo[name as keyof typeof alertInfo]} system={systems[0]} global overwriteExisting={overwriteExisting} initialAlertsState={alerts} idPrefix="settings-global" />)}
					</div>
				</>}
			</div>
		</div>
	)
}

async function test() {
	try { await pb.send("/api/picket/test-notification", { method: "POST" }); toast({ title: "Telegram test sent" }) }
	catch (error) { toast({ title: "Telegram test failed", description: (error as ClientResponseError).data?.message, variant: "destructive" }) }
}
