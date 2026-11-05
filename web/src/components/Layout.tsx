import { useQueryClient } from "@tanstack/react-query";
import {
  Boxes,
  KeyRound,
  LayoutDashboard,
  LogOut,
  MapPin,
  Package,
  Receipt,
  Ruler,
  ShoppingCart,
  Truck,
  UserRound,
  Users,
  Warehouse,
  type LucideIcon,
} from "lucide-react";
import { NavLink, Outlet } from "react-router";

import { can, roleLabel } from "../lib/permissions";
import { session, useSession, type SessionUser } from "../lib/session";

type Item = { to: string; label: string; icon: LucideIcon; end?: boolean; allow?: (u: SessionUser) => boolean };

const sections: { title?: string; items: Item[] }[] = [
  { items: [{ to: "/", label: "Dashboard", icon: LayoutDashboard, end: true }] },
  { title: "Sales", items: [{ to: "/orders", label: "Orders", icon: ShoppingCart }] },
  {
    title: "Fulfilment",
    items: [
      { to: "/deliveries", label: "Deliveries", icon: Truck },
      { to: "/warehouses", label: "Warehouses", icon: Warehouse },
    ],
  },
  { title: "Billing", items: [{ to: "/invoices", label: "Invoices", icon: Receipt, allow: can.seeInvoices }] },
  {
    title: "Master data",
    items: [
      { to: "/products", label: "Products", icon: Package },
      { to: "/customers", label: "Customers", icon: Users },
      { to: "/salespersons", label: "Salespersons", icon: UserRound },
      { to: "/regions", label: "Regions", icon: MapPin },
      { to: "/uoms", label: "Units", icon: Ruler },
      { to: "/users", label: "Users", icon: KeyRound, allow: can.manageUsers },
    ],
  },
];

function NavItem({ to, label, icon: Icon, end }: Item) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) =>
        "flex items-center gap-2.5 rounded-md px-2.5 py-1.5 text-sm font-medium whitespace-nowrap transition-colors " +
        (isActive ? "bg-slate-800 text-white" : "text-slate-300 hover:bg-slate-800/60 hover:text-white")
      }
    >
      <Icon className="size-4 shrink-0" aria-hidden />
      {label}
    </NavLink>
  );
}

function UserBox({ user }: { user: SessionUser }) {
  const qc = useQueryClient();
  const signOut = () => {
    session.clear();
    // Drop cached data so the next person to sign in never sees it.
    qc.clear();
  };
  return (
    <div className="flex items-center justify-between gap-2 border-t border-slate-800 px-4 py-3 md:mt-auto">
      <div className="min-w-0 text-sm">
        <p className="truncate font-medium text-white">{user.name}</p>
        <p className="text-xs text-slate-400">{roleLabel[user.role]}</p>
      </div>
      <button
        type="button"
        onClick={signOut}
        className="rounded-md p-1.5 text-slate-400 hover:bg-slate-800 hover:text-white"
        aria-label="Sign out"
        title="Sign out"
      >
        <LogOut className="size-4" aria-hidden />
      </button>
    </div>
  );
}

export function Layout() {
  const user = useSession()!.user; // RequireAuth guarantees a session here
  const visibleSections = sections
    .map((s) => ({ ...s, items: s.items.filter((i) => !i.allow || i.allow(user)) }))
    .filter((s) => s.items.length > 0);

  return (
    <div className="min-h-screen md:flex">
      <aside className="flex flex-col bg-slate-900 md:sticky md:top-0 md:h-screen md:w-56 md:shrink-0 md:overflow-y-auto">
        <div className="flex items-center gap-2 px-4 py-4 text-white">
          <Boxes className="size-5 text-indigo-400" aria-hidden />
          <span className="font-semibold tracking-tight">salesflow</span>
        </div>
        {/* A horizontal strip on small screens, a column on wider ones. */}
        <nav aria-label="Main" className="flex gap-1 overflow-x-auto px-3 pb-3 md:flex-col md:gap-4 md:overflow-visible">
          {visibleSections.map((section, i) => (
            <div key={i} className="flex gap-1 md:flex-col md:gap-0.5">
              {section.title && (
                <p className="hidden px-2.5 pb-1 text-xs font-semibold tracking-wide text-slate-500 uppercase md:block">{section.title}</p>
              )}
              {section.items.map((item) => (
                <NavItem key={item.to} {...item} />
              ))}
            </div>
          ))}
        </nav>
        <UserBox user={user} />
      </aside>
      <main className="min-w-0 flex-1 px-4 py-6 md:px-8">
        <div className="mx-auto max-w-6xl">
          <Outlet />
        </div>
      </main>
    </div>
  );
}
