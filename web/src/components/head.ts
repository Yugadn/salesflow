import type { ReactNode } from "react";

/** A table header cell; numeric columns right-align so they line up with their values. */
export type HeadCell = ReactNode | { label: ReactNode; numeric: true };

/** Marks a header as numeric: <Table head={["Product", num("Total")]} />. */
export const num = (label: ReactNode): HeadCell => ({ label, numeric: true });

export function isNumericHead(h: HeadCell): h is { label: ReactNode; numeric: true } {
  return typeof h === "object" && h !== null && "numeric" in h;
}
