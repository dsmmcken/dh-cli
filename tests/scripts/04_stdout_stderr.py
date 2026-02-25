# Test stdout and stderr separation
import sys

print("This goes to stdout")
sys.stderr.write("This goes to stderr\n")
print("Back to stdout")
