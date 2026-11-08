#!/usr/bin/env python3
"""Inspect the release tag boundary; mutate only with an explicit --apply."""

import argparse
import json
from pathlib import Path
import subprocess
import sys

REPOSITORY = "Annonator/CoPilot-PerUser"
POLICY_PATH = Path(__file__).resolve().parents[1] / ".github/release-tag-ruleset.json"


def api(path, *, method="GET", payload=None):
    args = ["gh", "api", "--method", method, path]
    if payload is not None:
        args += ["--input", "-"]
    result = subprocess.run(args, input=json.dumps(payload) if payload else None,
                            text=True, capture_output=True, check=True)
    return json.loads(result.stdout)


def normalized(policy, *, public_only=False):
    """Compare every security-relevant property, including the entire bypass list."""
    fields = ("name", "target", "enforcement", "bypass_actors", "conditions", "rules")
    result = {key: policy.get(key) for key in fields}
    for key in ("bypass_actors", "rules"):
        result[key] = sorted(result[key] or [], key=lambda item: json.dumps(item, sort_keys=True))
    if public_only:
        result.pop("bypass_actors")
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--check", action="store_true", help="Fail if the exact active policy is absent")
    mode.add_argument("--check-public", action="store_true", help="Check public restrictions with a read-only workflow token")
    mode.add_argument("--apply", action="store_true", help="Create/update this policy (requires admin approval)")
    args = parser.parse_args()
    desired = json.loads(POLICY_PATH.read_text())
    endpoint = f"repos/{REPOSITORY}/rulesets"
    # Paginate so an unrelated ruleset cannot hide the policy. Parent rulesets are
    # additive and cannot weaken this exact repository-owned restriction.
    summaries = []
    page = 1
    while True:
        batch = api(f"{endpoint}?per_page=100&page={page}&includes_parents=false")
        summaries.extend(batch)
        if len(batch) < 100:
            break
        page += 1
    matches = [item for item in summaries if item["name"] == desired["name"]]
    if len(matches) > 1:
        raise ValueError("Ambiguous release tag policy: duplicate ruleset names")
    current = api(f"{endpoint}/{matches[0]['id']}") if matches else None
    matches_policy = current is not None and normalized(current) == normalized(desired)
    if args.check or args.check_public:
        # GitHub hides bypass_actors from callers without Administration:write.
        # The workflow verifies public restrictions; an admin must separately
        # run --check to verify the entire boundary including bypass identities.
        if args.check_public:
            matches_policy = current is not None and normalized(current, public_only=True) == normalized(desired, public_only=True)
        if not matches_policy:
            raise ValueError("Release tag policy is missing or differs; review --apply before releasing")
        print("Public release tag restrictions match; admin --check must verify bypass actors" if args.check_public
              else "Release tag policy matches the required active restriction")
        return
    if not args.apply:
        print(json.dumps({"repository": REPOSITORY, "current": current,
                          "proposed": desired, "change_required": not matches_policy}, indent=2))
        return
    if not matches_policy:
        target = f"{endpoint}/{current['id']}" if current else endpoint
        applied = api(target, method="PUT" if current else "POST", payload=desired)
        verified = api(f"{endpoint}/{applied['id']}")
        if normalized(verified) != normalized(desired):
            raise ValueError("Applied policy did not match on readback")
    print("Release tag policy matches the required active restriction")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        print(f"Release tag policy check failed: {error}", file=sys.stderr)
        sys.exit(1)
