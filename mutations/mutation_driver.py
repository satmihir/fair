#!/usr/bin/env python3
"""
SAFE Mutation Testing Driver for FAIR Library

This script:
1. Parses mutations.jsonl to get all mutations
2. For each mutation:
   - Applies the diff to the codebase
   - Runs Go tests
   - Records whether tests caught the mutation
   - Safely undoes ONLY the applied changes (not our mutation files!)
3. Updates mutations.jsonl with test results
4. Reports mutation testing score

SAFETY: Only reverts changes to tracked source files, preserves mutation files
"""

import json
import re
import subprocess
import os
import sys
import shutil
from datetime import datetime
from pathlib import Path
import tempfile

class SafeMutationDriver:
    def __init__(self, mutations_dir: str = "/Users/mihirsathe/Documents/GitHub/fair/mutations"):
        self.mutations_dir = Path(mutations_dir)
        self.repo_root = self.mutations_dir.parent
        # Use enhanced mutations file instead of basic one
        self.mutations_file = self.mutations_dir / "mutations.jsonl"
        
        # Track files that were changed by mutations so we can safely revert them
        self.changed_files = set()
        
        # Verify we're in the right directory
        if not (self.repo_root / "go.mod").exists():
            raise FileNotFoundError(f"go.mod not found in {self.repo_root}. Wrong directory?")
        
        # Clean up any existing .bak files from previous runs
        self.cleanup_backup_files()
    
    def cleanup_backup_files(self):
        """Clean up any .bak files that might exist from previous operations."""
        try:
            for bak_file in self.repo_root.rglob("*.bak"):
                if bak_file.is_file():
                    bak_file.unlink()
                    print(f"🗑️  Cleaned up backup file: {bak_file.relative_to(self.repo_root)}")
        except Exception as e:
            print(f"Warning: Could not clean up backup files: {e}")
    
    def load_mutations(self):
        """Load all mutations from mutations.jsonl"""
        mutations = []
        if not self.mutations_file.exists():
            print(f"❌ Mutations file not found: {self.mutations_file}")
            return []
            
        with open(self.mutations_file, 'r') as f:
            for line in f:
                line = line.strip()
                if line:
                    mutations.append(json.loads(line))
        return mutations
    
    def save_mutations(self, mutations):
        """Save updated mutations back to mutations.jsonl"""
        with open(self.mutations_file, 'w') as f:
            for mutation in mutations:
                f.write(json.dumps(mutation) + '\n')
    
    def get_changed_files_from_diff(self, diff_file: str) -> list:
        """Extract the list of files that would be changed by a diff."""
        diff_path = self.mutations_dir / diff_file
        if not diff_path.exists():
            return []
        
        changed_files = []
        try:
            with open(diff_path, 'r') as f:
                for line in f:
                    if line.startswith('--- a/'):
                        # Extract file path from diff header
                        file_path = line[6:].strip()
                        changed_files.append(file_path)
        except Exception as e:
            print(f"Warning: Could not parse diff {diff_file}: {e}")
        
        return changed_files
    
    def apply_diff(self, diff_file: str) -> bool:
        """Apply a diff file to the codebase. Returns True if successful."""
        diff_path = self.mutations_dir / diff_file
        if not diff_path.exists():
            print(f"❌ Diff file not found: {diff_path}")
            return False
        
        # Track which files this diff will change
        files_to_change = self.get_changed_files_from_diff(diff_file)
        
        try:
            # Apply the diff from repo root
            result = subprocess.run(
                ["git", "apply", str(diff_path)],
                cwd=self.repo_root,
                capture_output=True,
                text=True,
                timeout=30
            )
            
            if result.returncode != 0:
                print(f"❌ Failed to apply diff {diff_file}:")
                print(f"   stdout: {result.stdout}")
                print(f"   stderr: {result.stderr}")
                return False
            
            # Track the files we changed
            self.changed_files.update(files_to_change)
            return True
            
        except subprocess.TimeoutExpired:
            print(f"❌ Timeout applying diff {diff_file}")
            return False
        except Exception as e:
            print(f"❌ Error applying diff {diff_file}: {e}")
            return False
    
    def revert_changes_safely(self) -> bool:
        """Safely revert ONLY the files we changed, preserving mutation files."""
        if not self.changed_files:
            return True
            
        try:
            # Only revert the specific files we know we changed
            files_to_revert = list(self.changed_files)
            
            if files_to_revert:
                print(f"   🔄 Reverting files: {', '.join(files_to_revert)}")
                result = subprocess.run(
                    ["git", "checkout", "HEAD", "--"] + files_to_revert,
                    cwd=self.repo_root,
                    capture_output=True,
                    text=True,
                    timeout=30
                )
                
                # Clean up any .bak files that might have been created
                for file_path in files_to_revert:
                    bak_file = self.repo_root / f"{file_path}.bak"
                    if bak_file.exists():
                        bak_file.unlink()
                        print(f"   🗑️  Cleaned up backup file: {file_path}.bak")
            
            # Clear the tracking set
            self.changed_files.clear()
            return True
            
        except Exception as e:
            print(f"❌ Error reverting changes: {e}")
            return False
    
    def run_tests(self) -> tuple[bool, str]:
        """
        Run Go tests. 
        Returns (tests_passed, output)
        """
        try:
            result = subprocess.run(
                ["go", "test", "./..."],
                cwd=self.repo_root,
                capture_output=True,
                text=True,
                timeout=120  # 2 minute timeout for tests
            )
            
            # Tests passed if return code is 0
            tests_passed = result.returncode == 0
            output = result.stdout + result.stderr
            
            return tests_passed, output
            
        except subprocess.TimeoutExpired:
            return False, "Test execution timed out (120s)"
        except Exception as e:
            return False, f"Error running tests: {e}"

    def compute_line_coverage(self) -> tuple[bool, str, str]:
        """
        Compute line coverage on a clean codebase.
        Returns (ok, total_percent_string, details_output)
        """
        cover_profile = self.repo_root / "coverage.out"
        try:
            # Generate coverage profile
            result = subprocess.run(
                ["go", "test", "-covermode=count", f"-coverprofile={cover_profile}", "./..."],
                cwd=self.repo_root,
                capture_output=True,
                text=True,
                timeout=240
            )
            if result.returncode != 0:
                return False, "", result.stdout + result.stderr

            # Summarize coverage
            cov = subprocess.run(
                ["go", "tool", "cover", f"-func={cover_profile}"],
                cwd=self.repo_root,
                capture_output=True,
                text=True,
                timeout=60
            )
            if cov.returncode != 0:
                return False, "", cov.stdout + cov.stderr

            total_pct = ""
            for line in cov.stdout.splitlines():
                # Expected format: "total: (statements)  67.5%"
                m = re.search(r"^total:\s*\(statements\)\s*([0-9.]+)%$", line.strip())
                if m:
                    total_pct = m.group(1) + "%"
                    break

            return (True if total_pct else False), total_pct, cov.stdout
        except subprocess.TimeoutExpired:
            return False, "", "Coverage computation timed out"
        except Exception as e:
            return False, "", f"Coverage error: {e}"
    
    def test_mutation(self, mutation: dict) -> dict:
        """
        Test a single mutation.
        Returns updated mutation dict with test results.
        """
        mutation_file = mutation["mutation_file"]
        description = mutation["description"]
        
        print(f"\n🧬 Testing mutation: {mutation_file}")
        print(f"   Description: {description[:80]}...")
        
        # Apply the mutation
        if not self.apply_diff(mutation_file):
            mutation.update({
                "tested_on": datetime.now().isoformat(),
                "caught_by_test": None,  # Could not test
                "test_output": "Failed to apply diff"
            })
            return mutation
        
        print(f"   ✅ Applied diff successfully")
        
        # Run tests
        tests_passed, test_output = self.run_tests()
        
        # Mutation is "caught" if tests failed (meaning the mutation broke something)
        caught_by_test = not tests_passed
        
        if caught_by_test:
            print(f"   🎯 CAUGHT: Tests failed - mutation was detected!")
        else:
            print(f"   ⚠️  MISSED: Tests passed - mutation went undetected!")
        
        # Safely revert changes
        if not self.revert_changes_safely():
            print(f"   ❌ Failed to revert changes!")
        else:
            print(f"   🔄 Safely reverted changes")
        
        # Update mutation with results (preserve existing enhanced metadata)
        mutation.update({
            "tested_on": datetime.now().isoformat(),
            "caught_by_test": caught_by_test,
            "test_output": test_output[:500] if test_output else ""  # Truncate long output
        })
        
        return mutation
    
    def run_baseline_tests(self) -> bool:
        """Run tests on clean codebase to ensure they pass before starting."""
        print("🧪 Running baseline tests to ensure clean state...")
        
        # Make sure we start clean
        self.revert_changes_safely()
        
        tests_passed, output = self.run_tests()
        
        if not tests_passed:
            print("❌ Baseline tests failed! Cannot proceed with mutation testing.")
            print("Test output:")
            print(output)
            return False
        
        print("✅ Baseline tests passed. Ready for mutation testing.")
        return True
    
    def run_all_mutations(self):
        """Run mutation testing on all mutations."""
        print("🚀 Starting FAIR Library SAFE Mutation Testing")
        print(f"📁 Repository: {self.repo_root}")
        print(f"📄 Mutations file: {self.mutations_file}")
        
        # Run baseline tests
        if not self.run_baseline_tests():
            return
        
        # Load mutations
        mutations = self.load_mutations()
        print(f"🧬 Found {len(mutations)} mutations to test")
        
        # Test each mutation
        tested_mutations = []
        caught_count = 0
        testable_count = 0
        
        for i, mutation in enumerate(mutations, 1):
            print(f"\n📊 Progress: {i}/{len(mutations)}")
            
            updated_mutation = self.test_mutation(mutation)
            tested_mutations.append(updated_mutation)
            
            # Track statistics
            if updated_mutation.get("caught_by_test") is not None:
                testable_count += 1
                if updated_mutation["caught_by_test"]:
                    caught_count += 1
        
        # Save results
        self.save_mutations(tested_mutations)
        
        # Report results
        print(f"\n" + "="*60)
        print(f"🎯 MUTATION TESTING RESULTS")
        print(f"="*60)
        print(f"Total mutations: {len(mutations)}")
        print(f"Testable mutations: {testable_count}")
        print(f"Mutations caught by tests: {caught_count}")
        
        if testable_count > 0:
            score = (caught_count / testable_count) * 100
            print(f"📈 Mutation score: {score:.1f}%")
            
            if score >= 80:
                print("🏆 Excellent! Strong test suite.")
            elif score >= 60:
                print("👍 Good test coverage.")
            elif score >= 40:
                print("⚠️  Moderate test coverage. Consider more tests.")
            else:
                print("❌ Weak test coverage. Significant testing gaps.")
        else:
            print("❌ No testable mutations found.")
        
        print(f"="*60)
        print(f"📄 Results saved to: {self.mutations_file}")
        print(f"🔍 Enhanced metadata preserved for all mutations")

        # Compute and report line coverage on a clean codebase
        try:
            self.revert_changes_safely()
            coverage_ok, coverage_pct, coverage_detail = self.compute_line_coverage()
            if coverage_ok:
                print(f"📐 Line coverage (clean baseline): {coverage_pct}")
            else:
                print("⚠️  Could not compute line coverage")
                if coverage_detail:
                    print(coverage_detail[:500])
        except Exception as e:
            print(f"⚠️  Coverage computation error: {e}")

def main():
    """Main entry point."""
    try:
        driver = SafeMutationDriver()
        driver.run_all_mutations()
    except KeyboardInterrupt:
        print("\n🛑 Mutation testing interrupted by user")
        # Try to safely revert any pending changes
        try:
            driver.revert_changes_safely()
        except:
            pass
        sys.exit(1)
    except Exception as e:
        print(f"❌ Fatal error: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()
