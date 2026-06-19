import { signIn } from "@/auth";

export function SignInButton() {
  return (
    <form
      action={async () => {
        "use server";
        await signIn("google", { redirectTo: "/" });
      }}
    >
      <button className="button button-primary" type="submit">
        Continue with Google
      </button>
    </form>
  );
}
