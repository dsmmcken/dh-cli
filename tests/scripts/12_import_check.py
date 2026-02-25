# Check what's available in the Deephaven environment
import deephaven

print(f"Deephaven version: {deephaven.__version__}")
print("\nAvailable modules:")
for name in sorted(dir(deephaven)):
    if not name.startswith('_'):
        print(f"  - {name}")
