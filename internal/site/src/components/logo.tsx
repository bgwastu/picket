export function Logo({ className }: { className?: string }) {
	return (
		<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 130 28" className={className} role="img" aria-label="Picket">
			<path fill="currentColor" d="M2 3h12c7 0 11 4 11 10s-4 10-11 10H8v4H2V3Zm6 5v10h6c3 0 5-2 5-5s-2-5-5-5H8Z" />
			<text x="30" y="22" fill="currentColor" fontFamily="Inter, sans-serif" fontSize="23" fontWeight="700" letterSpacing="-.8">Picket</text>
		</svg>
	)
}
