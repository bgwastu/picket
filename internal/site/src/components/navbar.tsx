import { BellIcon, MenuIcon, PlusIcon, SearchIcon } from "lucide-react"
import { lazy, Suspense, useState } from "react"
import { Button, buttonVariants } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { cn, runOnce } from "@/lib/utils"
import { AddSystemDialog } from "./add-system"
import { Logo } from "./logo"
import { ModeToggle } from "./mode-toggle"
import { $router, basePath, Link, navigate } from "./router"
import { Tooltip, TooltipContent, TooltipTrigger } from "./ui/tooltip"

const CommandPalette = lazy(() => import("./command-palette"))
const isMac = navigator.platform.toUpperCase().includes("MAC")

export default function Navbar() {
	const [addSystemOpen, setAddSystemOpen] = useState(false)
	const [commandPaletteOpen, setCommandPaletteOpen] = useState(false)

	return (
		<div className="flex items-center h-14 md:h-16 bg-card px-4 pe-3 sm:px-6 border border-border/60 rounded-md my-4">
			<Suspense><CommandPalette open={commandPaletteOpen} setOpen={setCommandPaletteOpen} /></Suspense>
			<AddSystemDialog open={addSystemOpen} setOpen={setAddSystemOpen} />
			<Link href={basePath} aria-label="Picket home" className="p-2 ps-0 me-3 group" onMouseEnter={runOnce(() => import("@/components/routes/home"))}>
				<Logo className="h-[1.2rem] md:h-5 text-foreground" />
			</Link>
			<Button variant="outline" className="hidden md:block text-sm text-muted-foreground px-4" onClick={() => setCommandPaletteOpen(true)}>
				<span className="flex items-center"><SearchIcon className="me-1.5 h-4 w-4" />Search <span className="flex items-center ms-3.5"><Kbd>{isMac ? "Cmd" : "Ctrl"}</Kbd><Kbd>K</Kbd></span></span>
			</Button>
			<div className="ms-auto flex items-center md:hidden">
				<ModeToggle />
				<Button variant="ghost" size="icon" aria-label="Search" onClick={() => setCommandPaletteOpen(true)}><SearchIcon className="size-5" /></Button>
				<DropdownMenu>
					<DropdownMenuTrigger className="ms-2" aria-label="Open menu"><MenuIcon /></DropdownMenuTrigger>
					<DropdownMenuContent align="end">
						<DropdownMenuItem onClick={() => navigate(getPagePath($router, "settings", { name: "notifications" }))}><BellIcon className="size-4 me-2.5" />Telegram Notifications</DropdownMenuItem>
						<DropdownMenuItem onSelect={() => setAddSystemOpen(true)}><PlusIcon className="size-4 me-2.5" />Add System</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
			</div>
			<div className="hidden md:flex items-center ms-auto">
				<NavIcon href={getPagePath($router, "settings", { name: "notifications" })} label="Telegram Notifications"><BellIcon className="size-5" /></NavIcon>
				<ModeToggle />
				<Button variant="outline" className="flex gap-1 ms-2" onClick={() => setAddSystemOpen(true)}><PlusIcon className="size-4" />Add System</Button>
			</div>
		</div>
	)
}

function NavIcon({ href, label, children }: { href: string; label: string; children: React.ReactNode }) {
	return <Tooltip><TooltipTrigger asChild><Link href={href} aria-label={label} className={cn(buttonVariants({ variant: "ghost", size: "icon" }))}>{children}</Link></TooltipTrigger><TooltipContent>{label}</TooltipContent></Tooltip>
}

const Kbd = ({ children }: { children: React.ReactNode }) => <kbd className="pointer-events-none inline-flex h-5 items-center rounded border bg-muted px-1.5 font-mono text-[10px] text-muted-foreground">{children}</kbd>
import { getPagePath } from "@nanostores/router"
