/** Returns the shortest decimal that reads back as the same float32, so a proto `float` field shows the value the server sent. */
export function float32(n: number): string {
	for (let digits = 1; digits <= 9; digits++) {
		const shortest = Number(n.toPrecision(digits));
		if (Math.fround(shortest) === n) return String(shortest);
	}
	return String(n);
}
