import { createRouter } from "@nanostores/router"
import type { AnchorHTMLAttributes, MouseEvent } from "react"

const routes = {
	home: "/",
	containers: "/containers",
	system: `/system/:id`,
	settings: `/settings/:name?`,
} as const

/**
 * The base path of the application.
 * This is used to prepend the base path to all routes.
 */
export const basePath = globalThis.PICKET?.BASE_PATH || ""

/**
 * Prepends the base path to the given path.
 * @param path The path to prepend the base path to.
 * @returns The path with the base path prepended.
 */
export const prependBasePath = (path: string) => (basePath + path).replaceAll("//", "/")

// prepend base path to routes
for (const route in routes) {
	// @ts-expect-error need as const above to get nanostores to parse types properly
	routes[route] = prependBasePath(routes[route])
}

export const $router = createRouter(routes, { links: false })

/** Navigate to url using router
 *  Base path is automatically prepended if serving from subpath
 */
export const navigate = (urlString: string) => {
	$router.open(urlString)
}

type LinkProps = Omit<AnchorHTMLAttributes<HTMLAnchorElement>, "href"> & { href: string }

export function Link(props: LinkProps) {
	function handleClick(event: MouseEvent<HTMLAnchorElement>) {
		if (props.onClick) props.onClick(event)
		if (event.defaultPrevented) return
		event.preventDefault()
		const href = props.href || ""
		if (event.ctrlKey || event.metaKey) {
			window.open(href, "_blank")
		} else {
			navigate(href)
		}
	}

	return (
		<a
			{...props}
			href={props.href}
			onClick={handleClick}
		>
			{props.children}
		</a>
	)
}
