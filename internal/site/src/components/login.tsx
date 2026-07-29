import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Logo } from "@/components/logo"
import { pb } from "@/lib/api"

export default function Login({ onAuthenticated }: { onAuthenticated: () => void }) {
	const [password, setPassword] = useState("")
	const [error, setError] = useState("")
	const [loading, setLoading] = useState(false)

	async function submit(event: React.FormEvent) {
		event.preventDefault()
		setLoading(true)
		setError("")
		try {
			await pb.send("/api/picket/auth", { method: "POST", body: { password } })
			onAuthenticated()
		} catch {
			setError("Incorrect password")
		} finally {
			setLoading(false)
		}
	}

	return (
		<div className="min-h-dvh grid place-items-center bg-muted/30 p-6">
			<Card className="w-full max-w-sm">
				<CardHeader className="items-center gap-4">
					<Logo className="h-7 w-auto" />
					<CardTitle>Sign in to Picket</CardTitle>
				</CardHeader>
				<CardContent>
					<form onSubmit={submit} className="grid gap-4">
						<div className="grid gap-2"><Label htmlFor="hub-password">Hub password</Label><Input id="hub-password" type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoFocus required /></div>
						{error && <p className="text-sm text-destructive">{error}</p>}
						<Button disabled={loading}>{loading ? "Signing in..." : "Sign in"}</Button>
					</form>
				</CardContent>
			</Card>
		</div>
	)
}
