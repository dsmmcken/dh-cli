# Combined test script that loads ALL complex test widgets.
# Start with: dh serve tests/scripts/complex/all_complex.py --port 10002

exec(open("tests/scripts/complex/test_dashboard.py").read())
exec(open("tests/scripts/complex/test_interactive.py").read())
exec(open("tests/scripts/complex/test_combined_dashboard.py").read())
exec(open("tests/scripts/complex/test_combined_counter.py").read())
exec(open("tests/scripts/complex/test_combined_error.py").read())
exec(open("tests/scripts/complex/test_iris_dashboard.py").read())
