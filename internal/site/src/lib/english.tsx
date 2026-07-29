import type { ReactNode } from "react"

type MessageDescriptor = { message: string }

export function t(strings: TemplateStringsArray | MessageDescriptor, ...values: unknown[]): string {
	if (!Array.isArray(strings)) return (strings as MessageDescriptor).message
	return strings.reduce((message, part, index) => message + part + (index < values.length ? String(values[index]) : ""), "")
}

export function Trans({ children }: { children?: ReactNode; context?: string; comment?: string }) {
	return children
}

export function Plural({ value, one, other }: { value: number; one: string; other: string }) {
	return (value === 1 ? one : other).replace("#", String(value))
}

export function plural(value: number, forms: { one: string; other: string; few?: string; many?: string }) {
	return (value === 1 ? forms.one : forms.other).replace("#", String(value))
}

export function useLingui() {
	return { t, i18n: { locale: "en" } }
}
