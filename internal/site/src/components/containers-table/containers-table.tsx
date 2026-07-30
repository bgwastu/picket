/** biome-ignore-all lint/security/noDangerouslySetInnerHtml: html comes directly from docker via agent */
import { t, Trans } from "@/lib/english"
import {
	type ColumnFiltersState,
	flexRender,
	getCoreRowModel,
	getFilteredRowModel,
	getSortedRowModel,
	type Row,
	type SortingState,
	type Table as TableType,
	useReactTable,
	type VisibilityState,
} from "@tanstack/react-table"
import { useVirtualizer, type VirtualItem } from "@tanstack/react-virtual"
import { memo, type RefObject, useEffect, useRef, useState } from "react"
import { Input } from "@/components/ui/input"
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { pb } from "@/lib/api"
import type { ContainerRecord } from "@/types"
import { containerChartCols } from "@/components/containers-table/containers-table-columns"
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { type ContainerHealth, ContainerHealthLabels } from "@/lib/enums"
import { cn, copyToClipboard, useBrowserStorage } from "@/lib/utils"
import { Sheet, SheetTitle, SheetHeader, SheetContent, SheetDescription } from "../ui/sheet"
import { Dialog, DialogContent, DialogTitle } from "../ui/dialog"
import { Button } from "@/components/ui/button"
import { $allSystemsById } from "@/lib/stores"
import { CheckIcon, CopyIcon, LoaderCircleIcon, MaximizeIcon, RefreshCwIcon, XIcon } from "lucide-react"
import { Separator } from "../ui/separator"
import { $router, Link } from "../router"
import { listenKeys } from "nanostores"
import { getPagePath } from "@nanostores/router"

export default function ContainersTable({ systemId }: { systemId?: string }) {
	const loadTime = Date.now()
	const [data, setData] = useState<ContainerRecord[] | undefined>(undefined)
	const [sorting, setSorting] = useBrowserStorage<SortingState>(
		`sort-c-${systemId ? 1 : 0}`,
		[{ id: systemId ? "name" : "system", desc: false }],
		sessionStorage
	)
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
	const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})

	// Hide ports column if no ports are present
	useEffect(() => {
		if (data) {
			const hasPorts = data.some((container) => container.ports)
			setColumnVisibility((prev) => {
				if (prev.ports === hasPorts) {
					return prev
				}
				return { ...prev, ports: hasPorts }
			})
		}
	}, [data])

	const [rowSelection, setRowSelection] = useState({})
	const [globalFilter, setGlobalFilter] = useState("")

	useEffect(() => {
		function fetchData(systemId?: string) {
			pb.collection<ContainerRecord>("containers")
				.getList(0, 2000, {
					fields: "id,name,image,ports,cpu,memory,net,health,status,system,updated",
					filter: systemId ? pb.filter("system={:system}", { system: systemId }) : undefined,
				})
				.then(({ items }) => {
					if (items.length === 0) {
						setData((curItems) => {
							if (systemId) {
								return curItems?.filter((item) => item.system !== systemId) ?? []
							}
							return []
						})
						return
					}
					setData((curItems) => {
						const lastUpdated = Math.max(items[0].updated, items.at(-1)?.updated ?? 0)
						const containerIds = new Set()
						const newItems: ContainerRecord[] = []
						for (const item of items) {
							if (Math.abs(lastUpdated - item.updated) < 70_000) {
								containerIds.add(item.id)
								newItems.push(item)
							}
						}
						for (const item of curItems ?? []) {
							if (!containerIds.has(item.id) && lastUpdated - item.updated < 70_000) {
								newItems.push(item)
							}
						}
						return newItems
					})
				})
		}

		// initial load
		fetchData(systemId)

		// if no systemId, pull system containers after every system update
		if (!systemId) {
			return $allSystemsById.listen((_value, _oldValue, systemId) => {
				// exclude initial load of systems
				if (Date.now() - loadTime > 500) {
					fetchData(systemId)
				}
			})
		}

		// if systemId, fetch containers after the system is updated
		return listenKeys($allSystemsById, [systemId], (_newSystems) => {
			fetchData(systemId)
		})
	}, [])

	const table = useReactTable({
		data: data ?? [],
		columns: containerChartCols.filter((col) => (systemId ? col.id !== "system" : true)),
		getCoreRowModel: getCoreRowModel(),
		getSortedRowModel: getSortedRowModel(),
		getFilteredRowModel: getFilteredRowModel(),
		onSortingChange: setSorting,
		onColumnFiltersChange: setColumnFilters,
		onColumnVisibilityChange: setColumnVisibility,
		onRowSelectionChange: setRowSelection,
		defaultColumn: {
			sortUndefined: "last",
			size: 100,
			minSize: 0,
		},
		state: {
			sorting,
			columnFilters,
			columnVisibility,
			rowSelection,
			globalFilter,
		},
		onGlobalFilterChange: setGlobalFilter,
		globalFilterFn: (row, _columnId, filterValue) => {
			const container = row.original
			const systemName = $allSystemsById.get()[container.system]?.name ?? ""
			const id = container.id ?? ""
			const name = container.name ?? ""
			const status = container.status ?? ""
			const healthLabel = ContainerHealthLabels[container.health as ContainerHealth] ?? ""
			const image = container.image ?? ""
			const ports = container.ports ?? ""
			const searchString = `${systemName} ${id} ${name} ${healthLabel} ${status} ${image} ${ports}`.toLowerCase()

			return (filterValue as string)
				.toLowerCase()
				.split(" ")
				.every((term) => searchString.includes(term))
		},
	})

	const rows = table.getRowModel().rows
	const visibleColumns = table.getVisibleLeafColumns()

	return (
		<Card className="@container w-full px-3 py-5 sm:py-6 sm:px-6">
			<CardHeader className="p-0 mb-3 sm:mb-4">
				<div className="grid md:flex gap-x-5 gap-y-3 w-full items-end">
					<div className="px-2 sm:px-1">
						<CardTitle className="mb-2">
							<Trans>All Containers</Trans>
						</CardTitle>
						<CardDescription className="flex">
							<Trans>Click on a container to view more information.</Trans>
						</CardDescription>
					</div>
					<div className="relative ms-auto w-full max-w-full md:w-64">
						<Input
							placeholder={t`Filter...`}
							value={globalFilter}
							onChange={(e) => setGlobalFilter(e.target.value)}
							className="ps-4 pe-10 w-full"
						/>
						{globalFilter && (
							<Button
								type="button"
								variant="ghost"
								size="icon"
								aria-label={t`Clear`}
								className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7 text-muted-foreground"
								onClick={() => setGlobalFilter("")}
							>
								<XIcon className="h-4 w-4" />
							</Button>
						)}
					</div>
				</div>
			</CardHeader>
			<div className="rounded-md">
				<AllContainersTable table={table} rows={rows} colLength={visibleColumns.length} data={data} />
			</div>
		</Card>
	)
}

const AllContainersTable = memo(function AllContainersTable({
	table,
	rows,
	colLength,
	data,
}: {
	table: TableType<ContainerRecord>
	rows: Row<ContainerRecord>[]
	colLength: number
	data: ContainerRecord[] | undefined
}) {
	// The virtualizer will need a reference to the scrollable container element
	const scrollRef = useRef<HTMLDivElement>(null)
	const activeContainer = useRef<ContainerRecord | null>(null)
	const [sheetOpen, setSheetOpen] = useState(false)
	const openSheet = (container: ContainerRecord) => {
		activeContainer.current = container
		setSheetOpen(true)
	}

	const virtualizer = useVirtualizer<HTMLDivElement, HTMLTableRowElement>({
		count: rows.length,
		estimateSize: () => 54,
		getScrollElement: () => scrollRef.current,
		overscan: 5,
	})
	const virtualRows = virtualizer.getVirtualItems()

	const paddingTop = Math.max(0, virtualRows[0]?.start ?? 0 - virtualizer.options.scrollMargin)
	const paddingBottom = Math.max(0, virtualizer.getTotalSize() - (virtualRows[virtualRows.length - 1]?.end ?? 0))

	return (
		<div
			className={cn(
				"h-min max-h-[calc(100dvh-17rem)] max-w-full relative overflow-auto border rounded-md",
				// don't set min height if there are less than 2 rows, do set if we need to display the empty state
				(!rows.length || rows.length > 2) && "min-h-50"
			)}
			ref={scrollRef}
		>
			{/* add header height to table size */}
			<div style={{ height: `${virtualizer.getTotalSize() + 48}px`, paddingTop, paddingBottom }}>
				<table className="text-sm w-full h-full text-nowrap">
					<ContainersTableHead table={table} />
					<TableBody>
						{rows.length ? (
							virtualRows.map((virtualRow) => {
								const row = rows[virtualRow.index]
								return <ContainerTableRow key={row.id} row={row} virtualRow={virtualRow} openSheet={openSheet} />
							})
						) : (
							<TableRow>
								<TableCell colSpan={colLength} className="h-37 text-center pointer-events-none">
									{data ? (
										<Trans>No results.</Trans>
									) : (
										<LoaderCircleIcon className="animate-spin size-10 opacity-60 mx-auto" />
									)}
								</TableCell>
							</TableRow>
						)}
					</TableBody>
				</table>
			</div>
			<ContainerSheet sheetOpen={sheetOpen} setSheetOpen={setSheetOpen} activeContainer={activeContainer} />
		</div>
	)
})

async function getLogs(container: ContainerRecord): Promise<string> {
	try {
		const logsResponse = await pb.send<{ logs: string }>("/api/picket/containers/logs", {
				system: container.system,
				container: container.id,
		})
		return logsResponse.logs ?? ""
	} catch (error) {
		console.error(error)
		return ""
	}
}

function ContainerSheet({
	sheetOpen,
	setSheetOpen,
	activeContainer,
}: {
	sheetOpen: boolean
	setSheetOpen: (open: boolean) => void
	activeContainer: RefObject<ContainerRecord | null>
}) {
	const [logsDisplay, setLogsDisplay] = useState<string>("")
	const [logsFullscreenOpen, setLogsFullscreenOpen] = useState<boolean>(false)
	const [isRefreshingLogs, setIsRefreshingLogs] = useState<boolean>(false)
	const [logsCopied, setLogsCopied] = useState(false)
	const logsContainerRef = useRef<HTMLDivElement>(null)

	const container = activeContainer.current

	function scrollLogsToBottom() {
		if (logsContainerRef.current) {
			logsContainerRef.current.scrollTo({ top: logsContainerRef.current.scrollHeight })
		}
	}

	const refreshLogs = async () => {
		if (!container) return
		setIsRefreshingLogs(true)
		const startTime = Date.now()

		try {
			setLogsDisplay(await getLogs(container))
			setTimeout(scrollLogsToBottom, 20)
		} catch (error) {
			console.error(error)
		} finally {
			// Ensure minimum spin duration of 800ms
			const elapsed = Date.now() - startTime
			const remaining = Math.max(0, 500 - elapsed)
			setTimeout(() => {
				setIsRefreshingLogs(false)
			}, remaining)
		}
	}

	useEffect(() => {
		setLogsDisplay("")
		if (!container) return
		;(async () => {
			setLogsDisplay(await getLogs(container))
			setTimeout(scrollLogsToBottom, 20)
		})()
	}, [container])

	if (!container) return null

	return (
		<>
			<LogsFullscreenDialog
				open={logsFullscreenOpen}
				onOpenChange={setLogsFullscreenOpen}
				logsDisplay={logsDisplay}
				containerName={container.name}
				onRefresh={refreshLogs}
				isRefreshing={isRefreshingLogs}
			/>
			<Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
				<SheetContent className="w-full sm:max-w-220 p-0">
					<SheetHeader>
						<SheetTitle>{container.name}</SheetTitle>
						<SheetDescription className="flex flex-wrap items-center gap-x-2 gap-y-1">
							<Link className="hover:underline" href={getPagePath($router, "system", { id: container.system })}>
								{$allSystemsById.get()[container.system]?.name ?? ""}
							</Link>
							<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
							{container.status}
							<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
							{container.image}
							<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
							{container.id}
							{/* {container.ports && (
								<>
									<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
									{container.ports}
								</>
							)} */}
							{/* <Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
							{ContainerHealthLabels[container.health as ContainerHealth]} */}
						</SheetDescription>
					</SheetHeader>
					<div className="flex h-full min-h-0 flex-col bg-muted/20">
						<div className="flex items-center justify-between border-y px-5 py-3">
							<div><h3 className="font-medium">{t`Logs`}</h3><p className="text-xs text-muted-foreground">Live output from {container.name}</p></div>
							<div className="flex items-center gap-1">
									<Button variant="ghost" size="icon" aria-label={logsCopied ? t`Copied` : t`Copy logs`} onClick={async () => { await copyToClipboard(logsDisplay); setLogsCopied(true); setTimeout(() => setLogsCopied(false), 1500) }} disabled={!logsDisplay}>{logsCopied ? <CheckIcon className="size-4 text-green-500" /> : <CopyIcon className="size-4" />}</Button>
							<Button
								variant="ghost"
								size="sm"
								onClick={refreshLogs}
								className="h-8 w-8 p-0"
								disabled={isRefreshingLogs}
							>
								<RefreshCwIcon
									className={`size-4 transition-transform duration-300 ${isRefreshingLogs ? "animate-spin" : ""}`}
								/>
							</Button>
							<Button variant="ghost" size="sm" onClick={() => setLogsFullscreenOpen(true)} className="h-8 w-8 p-0">
								<MaximizeIcon className="size-4" />
							</Button>
							</div>
						</div>
						<div ref={logsContainerRef} className="min-h-0 flex-1 overflow-auto bg-[#101317] p-4 font-mono text-[12px] leading-6 text-zinc-200">
							<LogLines logs={logsDisplay} />
						</div>
					</div>
				</SheetContent>
			</Sheet>
		</>
	)
}

function ContainersTableHead({ table }: { table: TableType<ContainerRecord> }) {
	return (
		<TableHeader className="sticky top-0 z-50 w-full border-b-2">
			{table.getHeaderGroups().map((headerGroup) => (
				<tr key={headerGroup.id}>
					{headerGroup.headers.map((header) => {
						return (
							<TableHead className="px-2" key={header.id} style={{ width: header.getSize() }}>
								{header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
							</TableHead>
						)
					})}
				</tr>
			))}
		</TableHeader>
	)
}

const ContainerTableRow = memo(function ContainerTableRow({
	row,
	virtualRow,
	openSheet,
}: {
	row: Row<ContainerRecord>
	virtualRow: VirtualItem
	openSheet: (container: ContainerRecord) => void
}) {
	return (
		<TableRow
			data-state={row.getIsSelected() && "selected"}
			className="cursor-pointer transition-opacity"
			onClick={() => openSheet(row.original)}
		>
			{row.getVisibleCells().map((cell) => (
				<TableCell
					key={cell.id}
					className="py-0 ps-4.5"
					style={{
						height: virtualRow.size,
						width: cell.column.getSize(),
					}}
				>
					{flexRender(cell.column.columnDef.cell, cell.getContext())}
				</TableCell>
			))}
		</TableRow>
	)
})

function LogLines({ logs }: { logs: string }) {
	if (!logs) {
		return <div className="py-10 text-center text-zinc-500">{t`No logs available.`}</div>
	}
	return (
		<div className="min-w-max">
			{logs.split("\n").map((line, index) => (
				<div className="flex min-h-6" key={`${index}-${line}`}>
					<span className="sticky left-0 w-12 shrink-0 select-none border-r border-white/10 bg-[#101317] pe-3 text-right text-zinc-600">{index + 1}</span>
					<span className="whitespace-pre px-4">{line || " "}</span>
				</div>
			))}
		</div>
	)
}

function LogsFullscreenDialog({
	open,
	onOpenChange,
	logsDisplay,
	containerName,
	onRefresh,
	isRefreshing,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	logsDisplay: string
	containerName: string
	onRefresh: () => void | Promise<void>
	isRefreshing: boolean
}) {
	const outerContainerRef = useRef<HTMLDivElement>(null)

	useEffect(() => {
		if (open && logsDisplay) {
			// Scroll the outer container to bottom
			const scrollToBottom = () => {
				if (outerContainerRef.current) {
					outerContainerRef.current.scrollTop = outerContainerRef.current.scrollHeight
				}
			}
			setTimeout(scrollToBottom, 50)
		}
	}, [open, logsDisplay])

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="w-[calc(100vw-20px)] h-[calc(100dvh-20px)] max-w-none p-0 bg-gh-dark border-0 text-white">
				<DialogTitle className="sr-only">{containerName} logs</DialogTitle>
				<div ref={outerContainerRef} className="h-full overflow-auto bg-[#101317] p-5 font-mono text-[12px] leading-6 text-zinc-200">
					<LogLines logs={logsDisplay} />
				</div>
				<button
					onClick={onRefresh}
					className="absolute top-3 right-11 opacity-60 hover:opacity-100 p-1"
					disabled={isRefreshing}
					title={t`Refresh`}
					aria-label={t`Refresh`}
				>
					<RefreshCwIcon className={`size-4 transition-transform duration-300 ${isRefreshing ? "animate-spin" : ""}`} />
				</button>
			</DialogContent>
		</Dialog>
	)
}
