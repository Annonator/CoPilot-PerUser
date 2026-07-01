import { redirect } from "next/navigation";
import Link from "next/link";

import { auth } from "@/auth";
import { SignOutButton } from "@/components/sign-out-button";
import { UsageDashboard } from "@/components/usage-dashboard";
import { createAppToken } from "@/lib/app-token";
import { getMonthlyUsage, UsageApiError } from "@/lib/api";
import type { MonthlyUsage } from "@/lib/usage-types";

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

function usageErrorMessage(error: unknown): string {
  if (error instanceof UsageApiError) {
    return `Usage data is unavailable right now. API status: ${error.status}.`;
  }

  return "Usage data is unavailable right now.";
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

  let usage: MonthlyUsage | undefined;
  let errorMessage: string | undefined;
  try {
    const token = await createAppToken({
      email: session.user.email,
      name: session.user.name
    });
    usage = await getMonthlyUsage({ token, year, month });
  } catch (error) {
    errorMessage = usageErrorMessage(error);
  }

  return (
    <>
      <nav className="topbar">
        <Link href="/" className="brand">
          Copilot AI Usage
        </Link>
        <SignOutButton />
      </nav>
      {errorMessage ? <UsageDashboard error={errorMessage} /> : <UsageDashboard usage={usage} />}
    </>
  );
}
