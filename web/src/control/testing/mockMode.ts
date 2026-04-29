export async function startMockApiIfNeeded() {
  if (import.meta.env.MODE !== "mock") return;

  const { startControlMocks } = await import("./browser");
  await startControlMocks();
}
