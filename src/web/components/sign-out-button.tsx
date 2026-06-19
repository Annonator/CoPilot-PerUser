import { signOut } from "@/auth";

export function SignOutButton() {
  return (
    <form
      action={async () => {
        "use server";
        await signOut({ redirectTo: "/login" });
      }}
    >
      <button className="button button-secondary" type="submit">
        Sign out
      </button>
    </form>
  );
}
