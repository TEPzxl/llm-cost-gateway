export default function HomePage() {
  return (
    <main className="grid min-h-screen place-items-center bg-slate-50 px-4 py-8 text-slate-900">
      <section
        aria-labelledby="product-title"
        className="w-full max-w-3xl rounded-lg border border-slate-200 bg-white p-6 shadow-sm sm:p-8"
      >
        <p className="mb-2 text-xs font-bold uppercase text-sky-700">
          Management Console
        </p>
        <h1 id="product-title" className="text-3xl font-semibold sm:text-4xl">
          LLM Cost Gateway
        </h1>
        <p className="mt-4 max-w-2xl text-base leading-7 text-slate-600">
          SaaS multi-tenant gateway for provider routing, usage metering, cost
          visibility, budget control, and request audit.
        </p>
        <dl className="mt-7 grid gap-3 sm:grid-cols-3">
          <div className="rounded-lg border border-slate-200 bg-slate-50 p-4">
            <dt className="text-xs font-bold uppercase text-slate-500">
              Backend
            </dt>
            <dd className="mt-1 text-sm font-semibold">Task 1 skeleton</dd>
          </div>
          <div className="rounded-lg border border-slate-200 bg-slate-50 p-4">
            <dt className="text-xs font-bold uppercase text-slate-500">
              Console
            </dt>
            <dd className="mt-1 text-sm font-semibold">Next.js placeholder</dd>
          </div>
          <div className="rounded-lg border border-slate-200 bg-slate-50 p-4">
            <dt className="text-xs font-bold uppercase text-slate-500">Next</dt>
            <dd className="mt-1 text-sm font-semibold">
              Database and migrations
            </dd>
          </div>
        </dl>
      </section>
    </main>
  );
}
