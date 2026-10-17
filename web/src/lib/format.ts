const dateFmt = new Intl.DateTimeFormat("en-GB", { dateStyle: "medium" });
const dateTimeFmt = new Intl.DateTimeFormat("en-GB", { dateStyle: "medium", timeStyle: "short" });

/** "17 Oct 2026", or with time "17 Oct 2026, 14:05". Dates without a time (YYYY-MM-DD) are shown as-is in local form. */
export function formatDate(iso: string | undefined | null, withTime = false): string {
  if (!iso) return "—";
  // A bare calendar date must not shift a day through time-zone conversion.
  const date = /^\d{4}-\d{2}-\d{2}$/.test(iso) ? new Date(`${iso}T00:00:00`) : new Date(iso);
  return (withTime ? dateTimeFmt : dateFmt).format(date);
}

/** Look-up helper: builds an id → item map from a list. */
export function byId<T extends { id: number }>(items: T[] | undefined): Map<number, T> {
  return new Map((items ?? []).map((i) => [i.id, i]));
}
