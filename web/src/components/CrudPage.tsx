// A list + create/edit modal for the simple master-data entities. Each page
// only declares its columns and form fields.

import type { UseMutationResult } from "@tanstack/react-query";
import { Pencil, Plus } from "lucide-react";
import { useState, type FormEvent, type ReactNode } from "react";

import { num } from "./head";
import { Button, Card, EmptyState, ErrorBanner, Field, Input, Modal, QueryState, Select, Table, Td } from "./ui";

type Form = Record<string, string>;

export type FieldDef = {
  name: string;
  label: string;
  type?: "text" | "number" | "select" | "password" | "boolean";
  options?: { value: string | number; label: string }[];
  hint?: string;
  /** Not required (an empty value is left out of the request). */
  optional?: boolean;
  /** Required on create, optional on edit (e.g. a password reset). */
  optionalOnEdit?: boolean;
  /** Shown but not editable once the record exists (e.g. a UoM code). */
  lockedOnEdit?: boolean;
  /** Not sent on update (the API ignores or forbids changing it). */
  createOnly?: boolean;
  /** Only shown, and sent, when editing an existing record. */
  editOnly?: boolean;
  /** Only shown, and sent, when this returns true for the current form. */
  showWhen?: (form: Form) => boolean;
};

export type Column<T> = { header: string; cell: (row: T) => ReactNode; numeric?: boolean };

// The form builds a plain object from its fields, so the page accepts any
// save mutation; each hook still types its own request body.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type SaveMutation = UseMutationResult<any, Error, any>;

type Props<T> = {
  title: string;
  description?: ReactNode;
  noun: string;
  query: { data?: T[]; isPending: boolean; error: unknown };
  columns: Column<T>[];
  fields: FieldDef[];
  rowKey: (row: T) => string | number;
  save: SaveMutation;
  /** Whether "New" is offered (the caller's role may create). */
  canCreate?: boolean;
  /** Whether rows can be edited (an edit endpoint exists and the role may use it). */
  editable?: boolean;
  rowActions?: (row: T) => ReactNode;
};

export function CrudPage<T extends object>({
  title,
  description,
  noun,
  query,
  columns,
  fields,
  rowKey,
  save,
  canCreate = true,
  editable = true,
  rowActions,
}: Props<T>) {
  const [editing, setEditing] = useState<T | "new" | null>(null);
  const [form, setForm] = useState<Form>({});
  const isNew = editing === "new";

  const visible = (f: FieldDef) => !(isNew && f.editOnly) && (!f.showWhen || f.showWhen(form));

  const open = (row: T | "new") => {
    save.reset();
    const initial: Form = {};
    for (const f of fields) {
      const value = row === "new" ? undefined : (row as Record<string, unknown>)[f.name];
      initial[f.name] = f.type === "password" || value === undefined || value === null ? "" : String(value);
    }
    setForm(initial);
    setEditing(row);
  };
  const close = () => setEditing(null);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const body: Record<string, unknown> = {};
    if (!isNew && editing) body.id = (editing as { id?: number }).id;
    for (const f of fields) {
      if (!visible(f) || (!isNew && f.createOnly)) continue;
      const raw = form[f.name] ?? "";
      if (raw === "" && (f.optional || (!isNew && f.optionalOnEdit))) continue;
      if (f.type === "boolean") body[f.name] = raw === "true";
      else if (f.type === "number" || (f.type === "select" && typeof f.options?.[0]?.value === "number")) body[f.name] = Number(raw);
      else body[f.name] = raw;
    }
    save.mutate(body, { onSuccess: close });
  };

  const rows = query.data ?? [];
  const showActions = editable || rowActions;

  return (
    <>
      <div className="mb-6 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-slate-900">{title}</h1>
          {description && <p className="mt-1 text-sm text-slate-500">{description}</p>}
        </div>
        {canCreate && (
          <Button onClick={() => open("new")}>
            <Plus className="size-4" aria-hidden /> New {noun}
          </Button>
        )}
      </div>

      <Card>
        <QueryState query={query}>
          <Table
            head={[...columns.map((c) => (c.numeric ? num(c.header) : c.header)), ...(showActions ? [""] : [])]}
            empty={rows.length === 0 && <EmptyState title={`No ${noun}s yet`} hint={canCreate ? `Create the first ${noun} to get started.` : undefined} />}
          >
            {rows.map((row) => (
              <tr key={rowKey(row)} className="hover:bg-slate-50">
                {columns.map((c) => (
                  <Td key={c.header} numeric={c.numeric}>
                    {c.cell(row)}
                  </Td>
                ))}
                {showActions && (
                  <Td className="text-right">
                    <div className="flex justify-end gap-1">
                      {rowActions?.(row)}
                      {editable && (
                        <Button variant="ghost" onClick={() => open(row)} aria-label={`Edit ${noun}`}>
                          <Pencil className="size-4" aria-hidden /> Edit
                        </Button>
                      )}
                    </div>
                  </Td>
                )}
              </tr>
            ))}
          </Table>
        </QueryState>
      </Card>

      <Modal
        open={editing !== null}
        title={isNew ? `New ${noun}` : `Edit ${noun}`}
        onClose={close}
        footer={
          <>
            <Button variant="secondary" onClick={close}>
              Cancel
            </Button>
            <Button type="submit" form="crud-form" loading={save.isPending}>
              {isNew ? "Create" : "Save"}
            </Button>
          </>
        }
      >
        <form id="crud-form" onSubmit={submit} className="space-y-4">
          {fields.filter(visible).map((f, i) => {
            const id = `field-${f.name}`;
            const locked = !isNew && (f.lockedOnEdit || f.createOnly);
            const common = {
              id,
              value: form[f.name] ?? "",
              disabled: locked,
              required: !f.optional && !(!isNew && f.optionalOnEdit),
              autoFocus: i === 0,
              onChange: (e: { target: { value: string } }) => setForm((prev) => ({ ...prev, [f.name]: e.target.value })),
            };
            return (
              <Field key={f.name} label={f.label} htmlFor={id} hint={locked ? "Cannot be changed after creation." : f.hint}>
                {f.type === "select" || f.type === "boolean" ? (
                  <Select {...common}>
                    <option value="" disabled>
                      Select…
                    </option>
                    {(f.type === "boolean"
                      ? [
                          { value: "true", label: "Active" },
                          { value: "false", label: "Inactive" },
                        ]
                      : (f.options ?? [])
                    ).map((o) => (
                      <option key={o.value} value={o.value}>
                        {o.label}
                      </option>
                    ))}
                  </Select>
                ) : (
                  <Input
                    {...common}
                    type={f.type === "number" ? "number" : f.type === "password" ? "password" : "text"}
                    autoComplete={f.type === "password" ? "new-password" : undefined}
                    min={f.type === "number" ? 0 : undefined}
                  />
                )}
              </Field>
            );
          })}
          <ErrorBanner error={save.error} />
        </form>
      </Modal>
    </>
  );
}
