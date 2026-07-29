import { useEffect } from "react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import Notifications from "./notifications"

export default function SettingsLayout() {
	useEffect(() => { document.title = "Telegram Notifications / Picket" }, [])
	return (
		<Card className="pt-5 px-4 pb-8 min-h-96 mb-14 sm:pt-6 sm:px-7">
			<CardHeader className="p-0 mb-5"><CardTitle>Telegram Notifications</CardTitle><CardDescription>Send Picket alerts to an allowed list of Telegram users.</CardDescription></CardHeader>
			<CardContent className="p-0"><Notifications /></CardContent>
		</Card>
	)
}
