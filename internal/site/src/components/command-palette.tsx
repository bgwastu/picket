import { getPagePath } from "@nanostores/router"
import { DialogDescription } from "@radix-ui/react-dialog"
import { BellIcon, Server, ServerIcon } from "lucide-react"
import { memo, useEffect } from "react"
import { CommandDialog, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList, CommandSeparator, CommandShortcut } from "@/components/ui/command"
import { $systems } from "@/lib/stores"
import { getHostDisplayValue, listen } from "@/lib/utils"
import { $router, basePath, navigate } from "./router"

export default memo(function CommandPalette({ open, setOpen }: { open: boolean; setOpen: (open: boolean) => void }) {
	useEffect(() => {
		const down = (event: KeyboardEvent) => {
			if (event.key === "k" && (event.metaKey || event.ctrlKey)) { event.preventDefault(); setOpen(!open) }
		}
		return listen(document, "keydown", down)
	}, [open, setOpen])
	const go = (path: string) => { navigate(path); setOpen(false) }
	return (
		<CommandDialog open={open} onOpenChange={setOpen}>
			<DialogDescription className="sr-only">Search Picket</DialogDescription>
			<CommandInput placeholder="Search systems or pages..." />
			<CommandList>
				<CommandGroup heading="Systems">{$systems.get().map((system) => <CommandItem key={system.id} onSelect={() => go(getPagePath($router, "system", { id: system.id }))}><Server className="me-2 size-4" /><span className="max-w-60 truncate">{system.name}</span><CommandShortcut>{getHostDisplayValue(system)}</CommandShortcut></CommandItem>)}</CommandGroup>
				<CommandSeparator />
				<CommandGroup heading="Pages">
					<CommandItem onSelect={() => go(basePath)}><ServerIcon className="me-2 size-4" />All Systems</CommandItem>
					<CommandItem onSelect={() => go(getPagePath($router, "settings", { name: "notifications" }))}><BellIcon className="me-2 size-4" />Telegram Notifications</CommandItem>
				</CommandGroup>
				<CommandEmpty>No results found.</CommandEmpty>
			</CommandList>
		</CommandDialog>
	)
})
