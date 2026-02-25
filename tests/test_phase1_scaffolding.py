"""Tests for Phase 1: Project scaffolding and package structure."""
import pytest


def test_package_imports():
    """Verify main package can be imported."""
    import deephaven_cli
    assert hasattr(deephaven_cli, '__version__')


def test_version_is_string():
    """Verify version is a valid string."""
    from deephaven_cli import __version__
    assert isinstance(__version__, str)
    assert len(__version__) > 0


def test_repl_subpackage_imports():
    """Verify repl subpackage can be imported."""
    from deephaven_cli import repl
    assert repl is not None


def test_cli_module_exists():
    """Verify cli module exists and has main function."""
    from deephaven_cli import cli
    assert hasattr(cli, 'main')
    assert callable(cli.main)


def test_cli_main_requires_subcommand():
    """Verify main() requires a subcommand."""
    from deephaven_cli.cli import main
    # With no args, argparse will raise SystemExit
    with pytest.raises(SystemExit):
        main()
