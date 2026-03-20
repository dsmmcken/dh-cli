# Combined error test script that loads ALL error test widgets.
# Start with: dh serve tests/scripts/errors/all_errors.py --port 10001

exec(open("tests/scripts/errors/err_none_access.py").read())
exec(open("tests/scripts/errors/err_index_out_of_range.py").read())
exec(open("tests/scripts/errors/err_type_mismatch.py").read())
exec(open("tests/scripts/errors/err_key_error.py").read())
exec(open("tests/scripts/errors/err_divide_by_zero.py").read())
exec(open("tests/scripts/errors/err_bad_callback.py").read())
exec(open("tests/scripts/errors/err_missing_prop.py").read())
exec(open("tests/scripts/errors/err_infinite_render.py").read())
exec(open("tests/scripts/errors/err_wrong_children.py").read())
exec(open("tests/scripts/errors/err_stale_closure.py").read())
exec(open("tests/scripts/errors/err_explicit_throw.py").read())
exec(open("tests/scripts/errors/err_import_missing.py").read())
exec(open("tests/scripts/errors/err_recursion.py").read())
exec(open("tests/scripts/errors/err_mutation_during_render.py").read())
exec(open("tests/scripts/errors/err_dialog_trigger_children.py").read())
