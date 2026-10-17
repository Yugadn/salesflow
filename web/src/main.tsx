import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { createBrowserRouter, Link } from "react-router";
import { RouterProvider } from "react-router/dom";

import { Layout } from "./components/Layout";
import { EmptyState } from "./components/ui";
import "./index.css";
import { DashboardPage } from "./pages/DashboardPage";
import { DeliveriesPage, DeliveryDetailPage } from "./pages/deliveries/DeliveryPages";
import { InvoiceDetailPage, InvoicesPage } from "./pages/invoices/InvoicePages";
import { CustomersPage, RegionsPage, SalespersonsPage, UoMsPage } from "./pages/master/MasterPages";
import { ProductDetailPage } from "./pages/master/ProductDetailPage";
import { ProductsPage } from "./pages/master/ProductsPage";
import { NewOrderPage } from "./pages/orders/NewOrderPage";
import { OrderDetailPage } from "./pages/orders/OrderDetailPage";
import { OrdersPage } from "./pages/orders/OrdersPage";
import { WarehousesPage, WarehouseStockPage } from "./pages/warehouse/WarehousePages";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5_000,
      // 4xx answers (not found, invalid) will not change on retry.
      retry: (count, err) => count < 2 && !(err instanceof Error && "status" in err && Number(err.status) < 500),
    },
  },
});

const router = createBrowserRouter([
  {
    element: <Layout />,
    children: [
      { index: true, element: <DashboardPage /> },
      { path: "orders", element: <OrdersPage /> },
      { path: "orders/new", element: <NewOrderPage /> },
      { path: "orders/:id", element: <OrderDetailPage /> },
      { path: "deliveries", element: <DeliveriesPage /> },
      { path: "deliveries/:id", element: <DeliveryDetailPage /> },
      { path: "warehouses", element: <WarehousesPage /> },
      { path: "warehouses/:id", element: <WarehouseStockPage /> },
      { path: "invoices", element: <InvoicesPage /> },
      { path: "invoices/:id", element: <InvoiceDetailPage /> },
      { path: "products", element: <ProductsPage /> },
      { path: "products/:id", element: <ProductDetailPage /> },
      { path: "customers", element: <CustomersPage /> },
      { path: "salespersons", element: <SalespersonsPage /> },
      { path: "regions", element: <RegionsPage /> },
      { path: "uoms", element: <UoMsPage /> },
      {
        path: "*",
        element: (
          <EmptyState
            title="Page not found"
            action={
              <Link to="/" className="text-sm text-indigo-700 hover:underline">
                Back to the dashboard
              </Link>
            }
          />
        ),
      },
    ],
  },
]);

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
);
