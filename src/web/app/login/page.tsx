import { redirect } from "next/navigation";

import { auth } from "@/auth";
import { SignInButton } from "@/components/sign-in-button";

export const dynamic = "force-dynamic";

export default async function LoginPage() {
  const session = await auth();
  if (session?.user?.email) {
    redirect("/");
  }

  return (
    <main className="login-shell">
      <section className="login-panel">
        <p className="eyebrow">Copilot AI credits</p>
        <h1>Sign in to view your usage</h1>
        <p>
          Use your company Google account. Access is limited to configured company email domains.
        </p>
        <SignInButton />
      </section>
    </main>
  );
}
