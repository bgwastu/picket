import { memo, useState } from "react"
import { Trans } from "@/lib/english"
import { compareSemVer, parseSemVer } from "@/lib/utils"
import type { GPUData } from "@/types"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import InfoBar from "./system/info-bar"
import { useSystemData } from "./system/use-system-data"
import { CpuChart, ContainerCpuChart } from "./system/charts/cpu-charts"
import { MemoryChart, ContainerMemoryChart, SwapChart } from "./system/charts/memory-charts"
import { RootDiskCharts, ExtraFsCharts } from "./system/charts/disk-charts"
import { BandwidthChart, ContainerNetworkChart } from "./system/charts/network-charts"
import { GpuPowerChart, GpuDetailCharts } from "./system/charts/gpu-charts"
import { LazyContainersTable } from "./system/lazy-tables"
import { LoadAverageChart } from "./system/charts/load-average-chart"
import { ContainerIcon, CpuIcon, HardDriveIcon } from "lucide-react"
import { GpuIcon } from "../ui/icons"
import ContainersTable from "../containers-table/containers-table"

const SEMVER_0_14_0 = parseSemVer("0.14.0")

export default memo(function SystemDetail({ id }: { id: string }) {
	const systemData = useSystemData(id)

	const {
		system,
		systemStats,
		containerData,
		chartData,
		containerChartConfigs,
		details,
		grid,
		setGrid,
		displayMode,
		setDisplayMode,
		activeTab,
		setActiveTab,
		mountedTabs,
		tabsRef,
		maxValues,
		isLongerChart,
		showMax,
		dataEmpty,
		isPodman,
		lastGpus,
		hasGpuData,
		hasGpuEnginesData,
		hasGpuPowerData,
	} = systemData

	if (!system.id) {
		return <div className="py-16 text-center text-muted-foreground">Loading agent...</div>
	}

	const hasContainers = containerData.length > 0
	const hasContainersTable = hasContainers && compareSemVer(chartData.agentVersion, SEMVER_0_14_0) >= 0
	const hasGpu = hasGpuData || hasGpuPowerData

	// keep tabsRef in sync for keyboard navigation
	const tabs = ["core", "disk"]
	if (hasGpu) tabs.push("gpu")
	if (hasContainers) tabs.push("containers")
	tabsRef.current = tabs

	// shared chart props
	const coreProps = { chartData, grid, dataEmpty, showMax, isLongerChart, maxValues }

	function defaultLayout() {
		return (
			<>
				{/* main charts */}
				<div className="grid xl:grid-cols-2 gap-4">
					<CpuChart {...coreProps} />

					{hasContainers && (
						<ContainerCpuChart
							chartData={chartData}
							grid={grid}
							dataEmpty={dataEmpty}
							isPodman={isPodman}
							cpuConfig={containerChartConfigs.cpu}
						/>
					)}

					<MemoryChart {...coreProps} />

					{hasContainers && (
						<ContainerMemoryChart
							chartData={chartData}
							grid={grid}
							dataEmpty={dataEmpty}
							isPodman={isPodman}
							memoryConfig={containerChartConfigs.memory}
						/>
					)}

					<RootDiskCharts systemData={systemData} />

					<BandwidthChart {...coreProps} systemStats={systemStats} />

					{hasContainers && (
						<ContainerNetworkChart
							chartData={chartData}
							grid={grid}
							dataEmpty={dataEmpty}
							isPodman={isPodman}
							networkConfig={containerChartConfigs.network}
						/>
					)}

					<SwapChart chartData={chartData} grid={grid} dataEmpty={dataEmpty} systemStats={systemStats} />

					<LoadAverageChart chartData={chartData} grid={grid} dataEmpty={dataEmpty} />


					{hasGpuPowerData && <GpuPowerChart chartData={chartData} grid={grid} dataEmpty={dataEmpty} />}
				</div>

				{hasGpuData && lastGpus && (
					<GpuDetailCharts
						chartData={chartData}
						grid={grid}
						dataEmpty={dataEmpty}
						lastGpus={lastGpus as Record<string, GPUData>}
						hasGpuEnginesData={hasGpuEnginesData}
					/>
				)}

				<ExtraFsCharts systemData={systemData} />

				{hasContainersTable && <LazyContainersTable systemId={system.id} />}

			</>
		)
	}

	function tabbedLayout() {
		return (
			<Tabs value={activeTab} onValueChange={setActiveTab} className="contents">
				<TabsList className="h-11 p-1.5 w-full shadow-xs overflow-auto justify-start">
					<TabsTrigger value="core" className="w-full flex items-center gap-1.5">
						<CpuIcon className="size-3.5" />
						<Trans context="Core system metrics">Core</Trans>
					</TabsTrigger>
					<TabsTrigger value="disk" className="w-full flex items-center gap-1.5">
						<HardDriveIcon className="size-3.5" />
						<Trans>Disk</Trans>
					</TabsTrigger>
					{hasGpu && (
						<TabsTrigger value="gpu" className="w-full flex items-center gap-2">
							<GpuIcon className="size-3.5" />
							<Trans>GPU</Trans>
						</TabsTrigger>
					)}
					{hasContainers && (
						<TabsTrigger value="containers" className="w-full flex items-center gap-2">
							<ContainerIcon className="size-3.5" />
							<Trans>Containers</Trans>
						</TabsTrigger>
					)}
				</TabsList>

				<TabsContent value="core" forceMount className={activeTab === "core" ? "contents" : "hidden"}>
					<div className="grid xl:grid-cols-2 gap-4">
						<CpuChart {...coreProps} />
						<MemoryChart {...coreProps} />
						<LoadAverageChart chartData={chartData} grid={grid} dataEmpty={dataEmpty} />
						<BandwidthChart {...coreProps} systemStats={systemStats} />
						<SwapChart chartData={chartData} grid={grid} dataEmpty={dataEmpty} systemStats={systemStats} />
					</div>
				</TabsContent>

				<TabsContent value="disk" forceMount className={activeTab === "disk" ? "contents" : "hidden"}>
					{mountedTabs.has("disk") && (
						<>
							<div className="grid xl:grid-cols-2 gap-4">
								<RootDiskCharts systemData={systemData} />
							</div>
							<ExtraFsCharts systemData={systemData} />
						</>
					)}
				</TabsContent>

				{hasGpu && (
					<TabsContent value="gpu" forceMount className={activeTab === "gpu" ? "contents" : "hidden"}>
						<div className="grid xl:grid-cols-2 gap-4">
							{hasGpuPowerData && <GpuPowerChart chartData={chartData} grid={grid} dataEmpty={dataEmpty} />}
						</div>
						{hasGpuData && lastGpus && (
							<GpuDetailCharts
								chartData={chartData}
								grid={grid}
								dataEmpty={dataEmpty}
								lastGpus={lastGpus as Record<string, GPUData>}
								hasGpuEnginesData={hasGpuEnginesData}
							/>
						)}
					</TabsContent>
				)}

				{hasContainers && (
					<TabsContent value="containers" forceMount className={activeTab === "containers" ? "contents" : "hidden"}>
						{mountedTabs.has("containers") && (
							<>
								<div className="grid xl:grid-cols-2 gap-4">
									<ContainerCpuChart
										chartData={chartData}
										grid={grid}
										dataEmpty={dataEmpty}
										isPodman={isPodman}
										cpuConfig={containerChartConfigs.cpu}
									/>
									<ContainerMemoryChart
										chartData={chartData}
										grid={grid}
										dataEmpty={dataEmpty}
										isPodman={isPodman}
										memoryConfig={containerChartConfigs.memory}
									/>
									<ContainerNetworkChart
										chartData={chartData}
										grid={grid}
										dataEmpty={dataEmpty}
										isPodman={isPodman}
										networkConfig={containerChartConfigs.network}
									/>
								</div>
								{hasContainersTable && <ContainersTable systemId={system.id} />}
							</>
						)}
					</TabsContent>
				)}

			</Tabs>
		)
	}

	return (
		<div className="grid gap-4 mb-14 overflow-x-clip">
			{/* system info */}
			<InfoBar
				system={system}
				chartData={chartData}
				grid={grid}
				setGrid={setGrid}
				displayMode={displayMode}
				setDisplayMode={setDisplayMode}
				details={details}
			/>

			{displayMode === "tabs" ? tabbedLayout() : defaultLayout()}
		</div>
	)
})
