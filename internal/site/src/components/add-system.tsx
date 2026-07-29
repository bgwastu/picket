import { LoaderCircleIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { InputCopy } from "@/components/ui/input-copy"
import { Label } from "@/components/ui/label"
import { toast } from "@/components/ui/use-toast"
import { pb } from "@/lib/api"
import type { SystemRecord } from "@/types"

type Enrollment = { system: SystemRecord; token: string }

export function AddSystemDialog({ open, setOpen }: { open: boolean; setOpen: (open: boolean) => void }) {
	return <Dialog open={open} onOpenChange={setOpen}>{open && <SystemDialog setOpen={setOpen} />}</Dialog>
}

export function SystemDialog({ setOpen, system }: { setOpen: (open: boolean) => void; system?: SystemRecord }) {
	const [name, setName] = useState(system?.name ?? "")
	const [token, setToken] = useState("")
	const [installScript, setInstallScript] = useState("")
	const [loading, setLoading] = useState(false)

	useEffect(() => { setName(system?.name ?? ""); setToken(""); setInstallScript("") }, [system])

	async function submit(event: React.FormEvent) {
		event.preventDefault()
		setLoading(true)
		try {
			if (system) {
				await pb.collection("systems").update(system.id, { name })
				setOpen(false)
				return
			}
			const enrollment = await pb.send<Enrollment>("/api/picket/systems", { method: "POST", body: { name } })
			setToken(enrollment.token)
			const installResponse = await fetch(`/api/picket/systems/${enrollment.system.id}/install-script`, { credentials: "same-origin" })
			if (!installResponse.ok) throw new Error("Unable to load install script")
			setInstallScript(await installResponse.text())
		} catch (error) {
			console.error(error)
			toast({ title: "Unable to create system", description: "The hub did not return an agent enrollment token.", variant: "destructive" })
		} finally {
			setLoading(false)
		}
	}

	return (
		<DialogContent className="w-[92%] sm:max-w-xl rounded-lg">
			<DialogHeader>
				<DialogTitle>{system ? "Edit System" : token ? "Connect Agent" : "Add System"}</DialogTitle>
				<DialogDescription>{token ? "The hub generated this one-time agent token. Start the agent with WebSocket connectivity to the hub." : "Name the agent and let the hub generate its enrollment token."}</DialogDescription>
			</DialogHeader>
			{token ? (
				<div className="space-y-4">
					<div className="grid gap-2"><Label>Linux installer</Label><InputCopy value={installScript} /></div>
					<p className="text-sm text-muted-foreground">Run this script as root or with sudo. It installs a native systemd daemon, connects it with the generated token, and keeps Docker monitoring available through the host socket.</p>
					<DialogFooter><Button onClick={() => setOpen(false)}>Close</Button></DialogFooter>
				</div>
			) : (
				<form onSubmit={submit} className="space-y-5">
					<div className="grid gap-2"><Label htmlFor="system-name">Name</Label><Input id="system-name" value={name} onChange={(event) => setName(event.target.value)} required autoFocus /></div>
					<DialogFooter><Button disabled={loading}>{loading && <LoaderCircleIcon className="size-4 animate-spin" />}{system ? "Save System" : "Generate Token"}</Button></DialogFooter>
				</form>
			)}
		</DialogContent>
	)
}
