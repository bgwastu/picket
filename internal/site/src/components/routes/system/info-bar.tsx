import { plural, Trans, useLingui } from "@/lib/english"
import {
	AppleIcon,
	ChevronRightSquareIcon,
	ClockArrowUp,
	CpuIcon,
	GlobeIcon,
	MemoryStickIcon,
	MoreHorizontalIcon,
	MonitorIcon,
	Settings2Icon,
} from "lucide-react"
import { useMemo, useState } from "react"
import ChartTimeSelect from "@/components/charts/chart-time-select"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { FreeBsdIcon, TuxIcon, WebSocketIcon, WindowsIcon } from "@/components/ui/icons"
import { Separator } from "@/components/ui/separator"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { ConnectionType, connectionTypeLabels, Os, SystemStatus } from "@/lib/enums"
import { cn, formatBytes, getHostDisplayValue, secondsToUptimeString, toFixedFloat } from "@/lib/utils"
import type { ChartData, SystemDetailsRecord, SystemRecord } from "@/types"
import { SystemDialog } from "@/components/add-system"
import { Dialog, DialogContent } from "@/components/ui/dialog"
import { Checkbox } from "@/components/ui/checkbox"
import { InputCopy } from "@/components/ui/input-copy"
import { pb } from "@/lib/api"
import { navigate } from "@/components/router"

export default function InfoBar({
	system,
	chartData,
	grid,
	setGrid,
	displayMode,
	setDisplayMode,
	details,
}: {
	system: SystemRecord
	chartData: ChartData
	grid: boolean
	setGrid: (grid: boolean) => void
	displayMode: "default" | "tabs"
	setDisplayMode: (mode: "default" | "tabs") => void
	details: SystemDetailsRecord | null
}) {
	const { t } = useLingui()
	const [editOpen, setEditOpen] = useState(false)
	const [installCommand, setInstallCommand] = useState("")
	const [installOpen, setInstallOpen] = useState(false)
	const [deleteOpen, setDeleteOpen] = useState(false)
	const [uninstallWithDelete, setUninstallWithDelete] = useState(false)
	const [deleteLoading, setDeleteLoading] = useState(false)
	const [sshOpen, setSshOpen] = useState(false)
	const [sshCommand, setSshCommand] = useState("")
	const [sshPrompt, setSshPrompt] = useState("")
	const [sshExpiresAt, setSshExpiresAt] = useState<string | null>(null)
	const [sshLoading, setSshLoading] = useState(false)

	async function createSSHLaunch() {
		setSshLoading(true)
		try {
			const response = await fetch(`/api/picket/systems/${system.id}/ssh-launch`, {
				method: "POST",
				credentials: "same-origin",
				headers: { "Content-Type": "application/json" },
			})
			if (!response.ok) throw new Error("Unable to create SSH launch")
			const launch = await response.json() as { token: string; expiresAt: string }
			const launchURL = `${window.location.origin}${globalThis.PICKET.BASE_PATH}api/picket/ssh-launch/${launch.token}`
			const command = `curl -fsSL '${launchURL}' | sh`
			setSshCommand(command)
			setSshPrompt(`Use this temporary Picket SSH access command to connect to ${system.name}. Run it in a trusted terminal, then use the resulting SSH session to inspect or operate the host. Access expires at ${new Date(launch.expiresAt).toLocaleString()}.\n\n${command}`)
			setSshExpiresAt(launch.expiresAt)
		} finally {
			setSshLoading(false)
		}
	}

	async function revokeSSHLaunch() {
		if (!sshCommand) return
		const token = sshCommand.match(/ssh-launch\/([^']+)/)?.[1]
		if (token) await fetch(`${globalThis.PICKET.BASE_PATH}api/picket/ssh-launch/${token}`, { method: "DELETE", credentials: "same-origin" })
		setSshCommand("")
		setSshPrompt("")
		setSshExpiresAt(null)
	}

	async function loadInstallScript() {
		const response = await fetch(`/api/picket/systems/${system.id}/install-command`, { credentials: "same-origin" })
		if (!response.ok) throw new Error("Unable to load install script")
		setInstallCommand(await response.text())
		setInstallOpen(true)
	}

	async function deleteSystem() {
		setDeleteLoading(true)
		try {
			if (uninstallWithDelete) {
				const response = await fetch(`/api/picket/systems/${system.id}/uninstall-agent`, { method: "POST", credentials: "same-origin" })
				if (!response.ok) throw new Error("Unable to uninstall agent")
			}
			await pb.collection("systems").delete(system.id)
			navigate("/")
		} finally {
			setDeleteLoading(false)
		}
	}

	// values for system info bar - use details with fallback to system.info
	const systemInfo = useMemo(() => {
		if (!system.info) {
			return []
		}

		// Use details if available, otherwise fall back to system.info
		const hostname = details?.hostname ?? system.info.h
		const kernel = details?.kernel ?? system.info.k
		const cores = details?.cores ?? system.info.c
		const threads = details?.threads ?? system.info.t ?? 0
		const cpuModel = details?.cpu ?? system.info.m
		const os = details?.os ?? system.info.os ?? Os.Linux
		const osName = details?.os_name
		const arch = details?.arch
		const memory = details?.memory

		const osInfo = {
			[Os.Linux]: {
				Icon: TuxIcon,
				// show kernel in tooltip if os name is available, otherwise show the kernel
				value: osName || kernel,
				label: osName ? kernel : undefined,
			},
			[Os.Darwin]: {
				Icon: AppleIcon,
				value: osName || `macOS ${kernel}`,
			},
			[Os.Windows]: {
				Icon: WindowsIcon,
				value: osName || kernel,
				label: osName ? kernel : undefined,
			},
			[Os.FreeBSD]: {
				Icon: FreeBsdIcon,
				value: osName || kernel,
				label: osName ? kernel : undefined,
			},
		}

		const info = [
			{ value: getHostDisplayValue(system), Icon: GlobeIcon },
			{
				value: hostname,
				Icon: MonitorIcon,
				label: "Hostname",
				// hide if hostname is the same as the system name
				hide: hostname === system.name,
			},
			{ value: secondsToUptimeString(system.info.u), Icon: ClockArrowUp, label: t`Uptime`, hide: !system.info.u },
			osInfo[os],
			{
				value: cpuModel,
				Icon: CpuIcon,
				hide: !cpuModel,
				label: `${plural(cores, { one: "# core", other: "# cores" })} / ${plural(threads, { one: "# thread", other: "# threads" })}${arch ? ` / ${arch}` : ""}`,
			},
		] as {
			value: string | number | undefined
			label?: string
			Icon: React.ElementType
			hide?: boolean
		}[]

		if (memory) {
			const memValue = formatBytes(memory, false, undefined, false)
			info.push({
				value: `${toFixedFloat(memValue.value, memValue.value >= 10 ? 1 : 2)} ${memValue.unit}`,
				Icon: MemoryStickIcon,
				hide: !memory,
				label: t`Memory`,
			})
		}

		return info
	}, [system, details, t])

	let translatedStatus: string = system.status
	if (system.status === SystemStatus.Up) {
		translatedStatus = t({ message: "Up", comment: "Context: System is up" })
	} else if (system.status === SystemStatus.Down) {
		translatedStatus = t({ message: "Down", comment: "Context: System is down" })
	}

	return (
		<Card>
			<div className="grid xl:flex xl:gap-4 px-4 sm:px-6 pt-3 sm:pt-4 pb-5">
				<div className="min-w-0">
					<h1 className="text-2xl sm:text-[1.6rem] font-semibold mb-1.5">{system.name}</h1>
					<div className="flex xl:flex-wrap items-center py-4 xl:p-0 -mt-3 xl:mt-1 gap-3 text-sm text-nowrap opacity-90 overflow-x-auto scrollbar-hide -mx-4 px-4 xl:mx-0">
						<Tooltip>
							<TooltipTrigger asChild>
								<div className="capitalize flex gap-2 items-center">
									<span className={cn("relative flex h-3 w-3")}>
										{system.status === SystemStatus.Up && (
											<span
												className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"
												style={{ animationDuration: "1.5s" }}
											></span>
										)}
										<span
											className={cn("relative inline-flex rounded-full h-3 w-3", {
												"bg-green-500": system.status === SystemStatus.Up,
												"bg-red-500": system.status === SystemStatus.Down,
												"bg-primary/40": system.status === SystemStatus.Paused,
												"bg-yellow-500": system.status === SystemStatus.Pending,
											})}
										></span>
									</span>
									{translatedStatus}
								</div>
							</TooltipTrigger>
							{system.info.ct && (
								<TooltipContent>
									<div className="flex gap-1 items-center">
										{system.info.ct === ConnectionType.WebSocket ? (
											<WebSocketIcon className="size-4" />
										) : (
											<ChevronRightSquareIcon className="size-4" strokeWidth={2} />
										)}
										{connectionTypeLabels[system.info.ct as ConnectionType]}
									</div>
								</TooltipContent>
							)}
						</Tooltip>

						{systemInfo.map(({ value, label, Icon, hide }) => {
							if (hide || !value) {
								return null
							}
							const content = (
								<div className="flex gap-1.5 items-center">
									<Icon className="h-4 w-4" /> {value}
								</div>
							)
							return (
								<div key={value} className="contents">
									<Separator orientation="vertical" className="h-4 bg-primary/30" />
									{label ? (
										<Tooltip delayDuration={100}>
											<TooltipTrigger asChild>{content}</TooltipTrigger>
											<TooltipContent>{label}</TooltipContent>
										</Tooltip>
									) : (
										content
									)}
								</div>
							)
						})}
					</div>
				</div>
				<div className="xl:ms-auto flex items-center gap-2 max-sm:-mb-1">
					<ChartTimeSelect className="w-full xl:w-40" agentVersion={chartData.agentVersion} />
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button
								aria-label={t`Settings`}
								variant="outline"
								size="icon"
								className="hidden xl:flex p-0 text-primary"
							>
								<Settings2Icon className="size-4 opacity-90" />
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end" className="min-w-44">
							<DropdownMenuLabel className="px-3.5">
								<Trans context="Layout display options">Display</Trans>
							</DropdownMenuLabel>
							<DropdownMenuSeparator />
							<DropdownMenuRadioGroup
								className="px-1 pb-1"
								value={displayMode}
								onValueChange={(v) => setDisplayMode(v as "default" | "tabs")}
							>
								<DropdownMenuRadioItem value="default" onSelect={(e) => e.preventDefault()}>
									<Trans context="Default system layout option">Default</Trans>
								</DropdownMenuRadioItem>
								<DropdownMenuRadioItem value="tabs" onSelect={(e) => e.preventDefault()}>
									<Trans context="Tabs system layout option">Tabs</Trans>
								</DropdownMenuRadioItem>
							</DropdownMenuRadioGroup>
							<DropdownMenuSeparator />
							<DropdownMenuLabel className="px-3.5">
								<Trans>Chart width</Trans>
							</DropdownMenuLabel>
							<DropdownMenuSeparator />
							<DropdownMenuRadioGroup
								className="px-1 pb-1"
								value={grid ? "grid" : "full"}
								onValueChange={(v) => setGrid(v === "grid")}
							>
								<DropdownMenuRadioItem value="grid" onSelect={(e) => e.preventDefault()}>
									<Trans>Grid</Trans>
								</DropdownMenuRadioItem>
								<DropdownMenuRadioItem value="full" onSelect={(e) => e.preventDefault()}>
									<Trans>Full</Trans>
								</DropdownMenuRadioItem>
							</DropdownMenuRadioGroup>
						</DropdownMenuContent>
					</DropdownMenu>
					<DropdownMenu>
						<DropdownMenuTrigger asChild><Button variant="outline" size="icon" aria-label="Agent actions"><MoreHorizontalIcon className="size-4" /></Button></DropdownMenuTrigger>
						<DropdownMenuContent align="end">
							<DropdownMenuLabel>Agent</DropdownMenuLabel>
							<DropdownMenuItem onSelect={() => setSshOpen(true)}>Connect over SSH</DropdownMenuItem>
							<DropdownMenuItem onSelect={() => setEditOpen(true)}>Rename agent</DropdownMenuItem>
							<DropdownMenuItem onSelect={loadInstallScript}>Show install script</DropdownMenuItem>
							<DropdownMenuSeparator />
							<DropdownMenuItem className="text-destructive" onSelect={() => setDeleteOpen(true)}>Delete agent</DropdownMenuItem>
						</DropdownMenuContent>
					</DropdownMenu>
				</div>
			</div>
			<Dialog open={editOpen} onOpenChange={setEditOpen}><SystemDialog setOpen={setEditOpen} system={system} /></Dialog>
			<Dialog open={installOpen} onOpenChange={setInstallOpen}>
				<DialogContent className="w-[92%] sm:max-w-2xl rounded-lg">
					<div className="space-y-4">
						<div><h2 className="text-lg font-semibold">Install this agent</h2><p className="text-sm text-muted-foreground">Run this one-line command as a user with sudo access on the Linux host.</p></div>
						<div className="grid gap-2"><p className="text-sm font-medium">Terminal command</p><InputCopy value={installCommand} /></div>
						<p className="text-sm text-muted-foreground">The command downloads the correct agent binary for the host architecture, installs the systemd service, and starts the agent.</p>
						<div className="flex justify-end"><Button variant="outline" onClick={() => setInstallOpen(false)}>Close</Button></div>
					</div>
				</DialogContent>
			</Dialog>
			<Dialog open={sshOpen} onOpenChange={setSshOpen}>
				<DialogContent className="w-[92%] sm:max-w-2xl rounded-lg">
					<div className="space-y-4">
						<div><h2 className="text-lg font-semibold">Temporary SSH access</h2><p className="text-sm text-muted-foreground">Generate a short-lived command for the account running Picket. SSH sessions close after 15 minutes without traffic.</p></div>
						{sshCommand ? <>
							<div className="space-y-2"><p className="text-sm font-medium">Terminal command</p><InputCopy value={sshCommand} /></div>
							{sshExpiresAt && <p className="text-sm text-muted-foreground">Access expires at {new Date(sshExpiresAt).toLocaleString()}.</p>}
							<div className="space-y-2"><p className="text-sm font-medium">Prompt for an AI agent</p><InputCopy value={sshPrompt} /></div>
							<div className="flex justify-end gap-2"><Button variant="destructive" onClick={revokeSSHLaunch}>Close access</Button><Button variant="outline" onClick={() => setSshOpen(false)}>Close</Button></div>
						</> : <div className="flex justify-end gap-2"><Button variant="outline" onClick={() => setSshOpen(false)}>Cancel</Button><Button onClick={createSSHLaunch} disabled={sshLoading}>{sshLoading ? "Generating..." : "Generate temporary access"}</Button></div>}
					</div>
				</DialogContent>
			</Dialog>
			<Dialog open={deleteOpen} onOpenChange={(open) => { setDeleteOpen(open); if (!open) setUninstallWithDelete(false) }}>
				<DialogContent className="w-[92%] sm:max-w-md rounded-lg">
					<div className="space-y-4">
						<div><h2 className="text-lg font-semibold">Delete {system.name}?</h2><p className="text-sm text-muted-foreground">This removes the agent and its collected data from Picket.</p></div>
						<label htmlFor="uninstall-agent-with-delete" className="flex items-start gap-3 rounded-md border p-3 text-sm">
							<Checkbox id="uninstall-agent-with-delete" checked={uninstallWithDelete} onCheckedChange={(checked) => setUninstallWithDelete(checked === true)} className="mt-0.5" />
							<span><span className="font-medium">Uninstall agent from host</span><span className="mt-1 block text-muted-foreground">Stop the Picket service and remove the agent files from the host before deleting this record.</span></span>
						</label>
						<div className="flex justify-end gap-2"><Button variant="outline" onClick={() => setDeleteOpen(false)} disabled={deleteLoading}>Cancel</Button><Button variant="destructive" onClick={deleteSystem} disabled={deleteLoading}>{deleteLoading ? "Deleting..." : "Delete"}</Button></div>
					</div>
				</DialogContent>
			</Dialog>
		</Card>
	)
}
