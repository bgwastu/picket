import { atom, computed, listenKeys, map, type ReadableAtom } from "nanostores"
import type { AlertMap, ChartTimes, DisplaySettings, SystemRecord } from "@/types"
import { Unit } from "./enums"

/** Default layout width. Used as fallback when user setting is unset. */
export const defaultLayoutWidth = 1580

/** Map of system records by name */
export const $allSystemsByName = map<Record<string, SystemRecord>>({})
/** Map of system records by id */
export const $allSystemsById = map<Record<string, SystemRecord>>({})
/** Map of up systems by id */
export const $upSystems = map<Record<string, SystemRecord>>({})
/** Map of down systems by id */
export const $downSystems = map<Record<string, SystemRecord>>({})
/** Map of paused systems by id */
export const $pausedSystems = map<Record<string, SystemRecord>>({})
/** List of all system records */
export const $systems: ReadableAtom<SystemRecord[]> = computed($allSystemsById, Object.values)

/** Map of alert records by system id and alert name */
export const $alerts = map<AlertMap>({})

/** Chart time period */
export const $chartTime = atom<ChartTimes>("1h")

/** Whether to display average or max chart values */
export const $maxValues = atom(false)

/** Fixed display defaults. These are intentionally not persisted. */
export const $displaySettings = map<DisplaySettings>({
	chartTime: "1h",
	unitNet: Unit.Bytes,
})
// update chart time on change
listenKeys($displaySettings, ["chartTime"], ({ chartTime }) => $chartTime.set(chartTime))

/** Container chart filter */
export const $containerFilter = atom("")


/** Fallback copy to clipboard dialog content */
export const $copyContent = atom("")

/** Longest system name length. Used to set table column width. I know this
 *  is stupid but the table is virtualized and I know this will work.
 */
export const $longestSystemNameLen = atom(8)
