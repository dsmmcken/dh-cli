# Slow script for testing timeout (use with --timeout=5)
import time

print("Starting slow operation...")
for i in range(10):
    print(f"Step {i + 1}/10")
    time.sleep(2)
print("Done!")
