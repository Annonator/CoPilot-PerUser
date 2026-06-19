import { redirect } from "next/navigation";

import { auth } from "@/auth";
import { SignOutButton } from "@/components/sign-out-button";
import { UsageDashboard } from "@/components/usage-dashboard";
import { createAppToken } from "@/lib/app-token";
import { getMonthlyUsage } from "@/lib/api";

type SearchParams = Record<string, string | string[] | undefined>;

export const dynamic = "force-dynamic";

function singleParam(value: string | string[] | undefined): string | undefined {
  return Array.isArray(value) ? value[0] : value;
}

function periodFromSearchParams(searchParams: SearchParams): { year: number; month: number } {
  const now = new Date();
  const year = Number(singleParam(searchParams.year)) || now.getUTCFullYear();
  const month = Number(singleParam(searchParams.month)) || now.getUTCMonth() + 1;

  return { year, month };
}

export default async function HomePage({
  searchParams
}: {
  searchParams?: Promise<SearchParams>;
}) {
  const session = await auth();
  if (!session?.user?.email) {
    redirect("/login");
  }

  const resolvedSearchParams = (await searchParams) ?? {};
  const { year, month } = periodFromSearchParams(resolvedSearchParams);

  let content: React.ReactNode;
  try {
    const token = await createAppToken({
      email: session.user.email,
      name: session.user.name
    });
    const usage = await getMonthlyUsage({ token, year, month });
    content = <UsageDashboard usage={usage} />;
  } catch (error) {
    content = <UsageDashboard error={error instanceof Error ? error.message : "Unknown error"} />;
  }

  return (
    <>
      <nav className="topbar">
        <a href="/" className="brand">
          Copilot AI Usage
        </a>
        <SignOutButton />
      </nav>
      {content}
    </>
  );
}
