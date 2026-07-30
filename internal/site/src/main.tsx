import "./index.css"
import { useStore } from "@nanostores/react"
// import { Suspense, lazy, useEffect, StrictMode } from "react"
import { lazy, memo, Suspense, useEffect, useState } from "react"
import ReactDOM from "react-dom/client"
import Navbar from "@/components/navbar.tsx"
import { $router } from "@/components/router.tsx"
import Settings from "@/components/routes/settings/layout.tsx"
import { ThemeProvider } from "@/components/theme-provider.tsx"
import { Toaster } from "@/components/ui/toaster.tsx"
import { alertManager } from "@/lib/alerts"
import { $copyContent, defaultLayoutWidth } from "@/lib/stores.ts"
import * as systemsManager from "@/lib/systemsManager.ts"
import Login from "@/components/login"

const Home = lazy(() => import("@/components/routes/home.tsx"))
const Containers = lazy(() => import("@/components/routes/containers.tsx"))
const SystemDetail = lazy(() => import("@/components/routes/system.tsx"))
const CopyToClipboardDialog = lazy(() => import("@/components/copy-to-clipboard.tsx"))

function isUnauthorized(error: unknown) {
	return (error as { status?: number })?.status === 401
}

const App = memo(() => {
	const page = useStore($router)

	useEffect(() => {
		// need to get system list before alerts
		systemsManager.init()
		systemsManager
			// get current systems list
			.refresh()
			// subscribe to new system updates
			.then(systemsManager.subscribe)
			// get current alerts
			.then(alertManager.refresh)
			// subscribe to new alert updates
			.then(alertManager.subscribe)
		return () => {
			alertManager.unsubscribe()
			systemsManager.unsubscribe()
		}
	}, [])

	if (!page) {
		return <h1 className="text-3xl text-center my-14">404</h1>
	} else if (page.route === "home") {
		return <Home />
	} else if (page.route === "system") {
		return <SystemDetail id={page.params.id} />
	} else if (page.route === "containers") {
		return <Containers />
	} else if (page.route === "settings") {
		return <Settings />
	}
})

const Layout = () => {
	const copyContent = useStore($copyContent)
	const [authenticated, setAuthenticated] = useState<boolean | null>(null)

	useEffect(() => {
		fetch(`${window.location.origin}/api/collections/systems/records?perPage=1`, { credentials: "include" })
			.then((response) => setAuthenticated(response.ok || !isUnauthorized({ status: response.status })))
			.catch(() => setAuthenticated(true))
	}, [])

	if (authenticated === false) return <Login onAuthenticated={() => setAuthenticated(true)} />
	if (authenticated === null) return null

	return (
		<div style={{ "--container": `${defaultLayoutWidth}px` } as React.CSSProperties}>
			<div className="container">
				<Navbar />
			</div>
			<div className="container relative">
				<App />
				{copyContent && (
					<Suspense>
						<CopyToClipboardDialog content={copyContent} />
					</Suspense>
				)}
			</div>
		</div>
	)
}

ReactDOM.createRoot(document.getElementById("app") as HTMLElement).render(
	// strict mode in dev mounts / unmounts components twice
	// and breaks the clipboard dialog
	//<StrictMode>
	<ThemeProvider>
		<Layout />
		<Toaster />
	</ThemeProvider>
	//</StrictMode>
)
