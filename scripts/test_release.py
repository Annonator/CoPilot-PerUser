"""Local release-boundary regressions: no GitHub writes or image publication."""

import contextlib
import copy
import importlib.util
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
POLICY = json.loads((ROOT / ".github/release-tag-ruleset.json").read_text())
spec = importlib.util.spec_from_file_location("release_policy", ROOT / "scripts/release-tag-policy.py")
policy_module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(policy_module)


class PolicyTests(unittest.TestCase):
    def invoke(self, mode, responses):
        with patch.object(sys, "argv", ["release-tag-policy.py", *mode]), \
                patch.object(policy_module, "api", side_effect=responses) as api, \
                contextlib.redirect_stdout(io.StringIO()) as output:
            policy_module.main()
            return api.call_args_list, output.getvalue()

    def test_dry_run_only_reads_and_shows_exact_payload(self):
        calls, output = self.invoke([], [[]])
        self.assertEqual(len(calls), 1)
        plan = json.loads(output)
        self.assertTrue(plan["change_required"])
        self.assertEqual(plan["proposed"], POLICY)
        self.assertEqual(POLICY["bypass_actors"], [
            {"actor_type": "User", "actor_id": 1070926, "bypass_mode": "always"}])
        self.assertEqual(POLICY["conditions"]["ref_name"], {"include": ["refs/tags/v*"], "exclude": []})
        self.assertEqual({item["type"] for item in POLICY["rules"]}, {"creation", "update", "deletion"})

    def test_missing_policy_fails_closed(self):
        with self.assertRaisesRegex(ValueError, "missing or differs"):
            self.invoke(["--check"], [[]])

    def test_workflow_token_can_verify_public_rules_without_hidden_bypass(self):
        visible = {key: value for key, value in POLICY.items() if key != "bypass_actors"}
        calls, output = self.invoke(["--check-public"], [[{"id": 1, "name": POLICY["name"]}], visible])
        self.assertEqual(len(calls), 2)
        self.assertIn("admin --check must verify bypass actors", output)
        with self.assertRaisesRegex(ValueError, "missing or differs"):
            self.invoke(["--check"], [[{"id": 1, "name": POLICY["name"]}], visible])

    def test_disabled_policy_extra_bypass_or_exclusion_fails_closed(self):
        changes = [
            {"enforcement": "disabled"},
            {"bypass_actors": POLICY["bypass_actors"] + [{"actor_type": "RepositoryRole", "actor_id": 4, "bypass_mode": "always"}]},
            {"conditions": {"ref_name": {"include": ["refs/tags/v*"], "exclude": ["refs/tags/v9*"]}}},
            {"rules": [rule for rule in POLICY["rules"] if rule["type"] != "creation"]},
        ]
        for change in changes:
            with self.subTest(change=change), self.assertRaisesRegex(ValueError, "missing or differs"):
                self.invoke(["--check"], [[{"id": 1, "name": POLICY["name"]}], {**POLICY, **change}])

    def test_existing_matching_policy_is_read_only_even_with_apply(self):
        for mode in (["--check"], ["--apply"]):
            calls, _ = self.invoke(mode, [[{"id": 1, "name": POLICY["name"]}], {"id": 1, **POLICY}])
            self.assertEqual(len(calls), 2)
            self.assertTrue(all(not call.kwargs for call in calls))

    def test_create_and_readback_then_update_and_readback(self):
        calls, _ = self.invoke(["--apply"], [[], {"id": 2}, {"id": 2, **POLICY}])
        self.assertEqual(calls[1].kwargs, {"method": "POST", "payload": POLICY})
        calls, _ = self.invoke(["--apply"], [
            [{"id": 2, "name": POLICY["name"]}], {"id": 2, **POLICY, "enforcement": "disabled"},
            {"id": 2}, {"id": 2, **POLICY},
        ])
        self.assertEqual(calls[2].kwargs, {"method": "PUT", "payload": POLICY})
        self.assertTrue(calls[2].args[0].endswith("/2"))

    def test_duplicate_names_and_bad_apply_readback_fail(self):
        with self.assertRaisesRegex(ValueError, "duplicate"):
            self.invoke(["--apply"], [[{"id": 1, "name": POLICY["name"]}] * 2])
        with self.assertRaisesRegex(ValueError, "readback"):
            self.invoke(["--apply"], [[], {"id": 1}, {**POLICY, "enforcement": "disabled"}])

    def test_pagination_cannot_hide_policy(self):
        calls, _ = self.invoke(["--check"], [
            [{"id": number, "name": "unrelated"} for number in range(100)],
            [{"id": 101, "name": POLICY["name"]}], POLICY,
        ])
        self.assertIn("page=2", calls[1].args[0])


class ReleaseSourceTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="release-boundary-")
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        self.repo = self.base / "checkout"
        self.repo.mkdir()
        self.env = {**os.environ, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull,
                    "GIT_AUTHOR_NAME": "Fixture", "GIT_COMMITTER_NAME": "Fixture",
                    "GIT_AUTHOR_EMAIL": "fixture@example.invalid", "GIT_COMMITTER_EMAIL": "fixture@example.invalid"}
        self.git("init", "-b", "main")
        (self.repo / "scripts").mkdir()
        (self.repo / ".github").mkdir()
        shutil.copy(ROOT / "scripts/release-tag-policy.py", self.repo / "scripts")
        shutil.copy(ROOT / ".github/release-tag-ruleset.json", self.repo / ".github")
        self.git("add", ".")
        self.git("commit", "-m", "Trusted main")
        self.sha = self.git("rev-parse", "HEAD")
        self.git("init", "--bare", str(self.base / "origin.git"))
        self.git("remote", "add", "origin", str(self.base / "origin.git"))
        self.git("push", "origin", "main")
        self.git("tag", "v1.2.3")
        self.git("push", "origin", "refs/tags/v1.2.3")
        bin_path = self.base / "bin"
        bin_path.mkdir()
        gh = bin_path / "gh"
        gh.write_text("#!/usr/bin/env python3\nimport json, os, sys\n"
                      "policy=json.load(open(os.environ['RELEASE_POLICY_FIXTURE']))\n"
                      "policy.pop('bypass_actors', None)\n"
                      "assert sys.argv[1:4] == ['api', '--method', 'GET'], 'writes prohibited'\n"
                      "print(json.dumps([dict(id=1,name=policy['name'])] if '?' in sys.argv[-1] else policy))\n")
        gh.chmod(0o755)
        self.fixture_policy = self.base / "server-policy.json"
        self.fixture_policy.write_text(json.dumps(POLICY))
        self.env.update(PATH=f"{bin_path}{os.pathsep}{self.env['PATH']}",
                        RELEASE_POLICY_FIXTURE=str(self.fixture_policy),
                        GITHUB_EVENT_NAME="workflow_dispatch", GITHUB_REF="refs/heads/main",
                        GITHUB_SHA=self.sha, RELEASE_TAG="v1.2.3")
        self.env.pop("GITHUB_OUTPUT", None)
        self.env.pop("EXPECTED_RELEASE_SHA", None)

    def git(self, *args):
        return subprocess.run(["git", *args], cwd=self.repo, env=self.env,
                              check=True, text=True, capture_output=True).stdout.strip()

    def verify(self, **changes):
        return subprocess.run(["bash", str(ROOT / "scripts/verify-release.sh")],
                              cwd=self.repo, env={**self.env, **changes}, text=True, capture_output=True)

    def assert_rejected(self, **changes):
        result = self.verify(**changes)
        self.assertNotEqual(result.returncode, 0, result.stdout)
        return result

    def test_lightweight_tag_and_immutable_outputs(self):
        output = self.base / "outputs"
        result = self.verify(GITHUB_OUTPUT=str(output))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(output.read_text(), f"sha={self.sha}\ntag=v1.2.3\nversion=1.2.3\n")

    def test_annotated_and_nested_annotated_tags_are_peeled(self):
        self.git("tag", "-a", "v2.0.0", "-m", "Annotated release")
        self.git("tag", "-a", "v2.0.1", "v2.0.0", "-m", "Nested annotated release")
        for tag in ("v2.0.0", "v2.0.1"):
            self.git("push", "origin", f"refs/tags/{tag}")
            result = self.verify(RELEASE_TAG=tag)
            self.assertEqual(result.returncode, 0, result.stderr)

    def test_unmerged_commit_and_ancestor_are_rejected(self):
        self.git("checkout", "-b", "unreviewed")
        self.git("commit", "--allow-empty", "-m", "Unreviewed")
        attacker = self.git("rev-parse", "HEAD")
        self.git("tag", "v9.0.0")
        self.git("push", "origin", "refs/tags/v9.0.0")
        self.git("checkout", "main")
        self.assert_rejected(RELEASE_TAG="v9.0.0")
        self.assert_rejected(RELEASE_TAG="v9.0.0", GITHUB_SHA=attacker)
        self.git("commit", "--allow-empty", "-m", "New main")
        current = self.git("rev-parse", "HEAD")
        self.git("push", "origin", "main")
        self.assert_rejected()  # stale/rerun workflow commit
        self.assert_rejected(GITHUB_SHA=current)  # old tag is only an ancestor

    def test_invalid_semver_injection_and_wrong_event_or_ref(self):
        for tag in ("v1", "v1.2", "v01.2.3", "v1.02.3", "v1.2.03", "v1.2.3-rc.1", "v1.2.3+build",
                    "v1.2.3\nsha=evil", "v1.2.3;touch PWNED", "$(touch PWNED)", "--upload-pack=evil", "v1.2.3^{}"):
            with self.subTest(tag=tag):
                self.assert_rejected(RELEASE_TAG=tag)
        self.assertFalse((self.repo / "PWNED").exists())
        self.assert_rejected(GITHUB_EVENT_NAME="push", GITHUB_REF="refs/tags/v1.2.3")
        self.assert_rejected(GITHUB_REF="refs/heads/unreviewed")
        self.assert_rejected(GITHUB_REF="refs/tags/v1.2.3")
        self.assert_rejected(EXPECTED_RELEASE_SHA="f" * 40)

    def test_missing_deleted_moved_and_noncommit_tag(self):
        self.assert_rejected(RELEASE_TAG="v8.0.0")
        self.git("tag", "v3.0.0", self.git("rev-parse", "HEAD^{tree}"))
        self.git("push", "origin", "refs/tags/v3.0.0")
        self.assert_rejected(RELEASE_TAG="v3.0.0")
        self.git("commit", "--allow-empty", "-m", "Retarget")
        self.git("tag", "-f", "v1.2.3")
        self.git("push", "--force", "origin", "refs/tags/v1.2.3")
        self.git("checkout", self.sha)
        self.assert_rejected()
        self.git("push", "origin", ":refs/tags/v1.2.3")
        self.assert_rejected()  # cached local tag cannot hide deletion

    def test_missing_server_boundary_blocks_even_trusted_source(self):
        changed = copy.deepcopy(POLICY)
        changed["enforcement"] = "disabled"
        self.fixture_policy.write_text(json.dumps(changed))
        result = self.assert_rejected()
        self.assertIn("policy is missing or differs", result.stderr)


if __name__ == "__main__":
    unittest.main()
