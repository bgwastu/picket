import { memo, Suspense, useEffect, useMemo } from "react"
import SystemsTable from "@/components/systems-table/systems-table"
import { ActiveAlerts } from "@/components/active-alerts"

export default memo(() => {
	useEffect(() => {
		document.title = "All Systems / Picket"
	}, [])

	return useMemo(
		() => (
			<>
				<div className="flex flex-col gap-4">
					<ActiveAlerts />
					<Suspense>
						<SystemsTable />
					</Suspense>
				</div>
			</>
		),
		[]
	)
})
