"""Unit tests for REPL prompt components."""
from __future__ import annotations

from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest
from prompt_toolkit.enums import EditingMode
from prompt_toolkit.formatted_text import HTML
from prompt_toolkit.key_binding import KeyBindings
from prompt_toolkit.keys import Keys


class TestToolbar:
    """Tests for toolbar.py."""

    def test_toolbar_returns_callable(self):
        """create_toolbar returns a callable."""
        from deephaven_cli.repl.prompt.toolbar import create_toolbar

        client = MagicMock()
        client.tables = ["table1", "table2"]
        toolbar_func = create_toolbar(client, 10000)
        assert callable(toolbar_func)

    def test_toolbar_shows_connected_status(self):
        """Toolbar shows 'Connected' when client works."""
        from deephaven_cli.repl.prompt.toolbar import create_toolbar

        client = MagicMock()
        client.tables = ["table1", "table2", "table3"]
        toolbar_func = create_toolbar(client, 8080)

        with patch("resource.getrusage") as mock_rusage:
            mock_rusage.return_value.ru_maxrss = 102400  # 100 MB
            result = toolbar_func()

        assert isinstance(result, HTML)
        html_str = result.value
        assert "Connected" in html_str
        assert "Port: 8080" in html_str
        assert "Tables: 3" in html_str
        assert "Mem:" in html_str

    def test_toolbar_shows_disconnected_on_error(self):
        """Toolbar shows 'Disconnected' when client fails."""
        from deephaven_cli.repl.prompt.toolbar import create_toolbar

        client = MagicMock()
        client.tables = property(lambda self: (_ for _ in ()).throw(Exception("conn error")))
        # Simulate tables raising an exception
        type(client).tables = property(lambda self: (_ for _ in ()).throw(Exception()))

        toolbar_func = create_toolbar(client, 10000)
        result = toolbar_func()

        assert isinstance(result, HTML)
        assert "Disconnected" in result.value


class TestKeyBindings:
    """Tests for keybindings.py."""

    def test_create_key_bindings_returns_keybindings(self):
        """create_key_bindings returns a KeyBindings object."""
        from deephaven_cli.repl.prompt.keybindings import create_key_bindings

        bindings = create_key_bindings()
        assert isinstance(bindings, KeyBindings)

    def test_ctrl_l_binding_exists(self):
        """Ctrl+L binding is registered."""
        from deephaven_cli.repl.prompt.keybindings import create_key_bindings

        bindings = create_key_bindings()
        # Check that there's at least one binding registered
        assert len(bindings.bindings) > 0

        # Verify Ctrl+L is bound
        ctrl_l_found = False
        for binding in bindings.bindings:
            if Keys.ControlL in binding.keys:
                ctrl_l_found = True
                break
        assert ctrl_l_found, "Ctrl+L binding not found"

    def test_enter_binding_exists(self):
        """Enter binding is registered for submit."""
        from deephaven_cli.repl.prompt.keybindings import create_key_bindings

        bindings = create_key_bindings()
        enter_found = False
        for binding in bindings.bindings:
            if Keys.Enter in binding.keys:
                enter_found = True
                break
        assert enter_found, "Enter binding not found"

    def test_alt_enter_binding_exists(self):
        """Alt+Enter (Escape+Enter) binding is registered for newline."""
        from deephaven_cli.repl.prompt.keybindings import create_key_bindings

        bindings = create_key_bindings()
        alt_enter_found = False
        for binding in bindings.bindings:
            if Keys.Escape in binding.keys and Keys.Enter in binding.keys:
                alt_enter_found = True
                break
        assert alt_enter_found, "Alt+Enter (Escape+Enter) binding not found"


class TestSession:
    """Tests for session.py."""

    def test_create_prompt_session_returns_session(self):
        """create_prompt_session returns a PromptSession."""
        from prompt_toolkit import PromptSession
        from deephaven_cli.repl.prompt.session import create_prompt_session

        client = MagicMock()
        client.tables = []
        session = create_prompt_session(client, 10000)

        assert isinstance(session, PromptSession)

    def test_session_uses_emacs_mode_by_default(self):
        """Session uses Emacs editing mode by default."""
        from deephaven_cli.repl.prompt.session import create_prompt_session

        client = MagicMock()
        client.tables = []
        session = create_prompt_session(client, 10000)

        assert session.editing_mode == EditingMode.EMACS

    def test_session_uses_vi_mode_when_requested(self):
        """Session uses Vi editing mode when vi_mode=True."""
        from deephaven_cli.repl.prompt.session import create_prompt_session

        client = MagicMock()
        client.tables = []
        session = create_prompt_session(client, 10000, vi_mode=True)

        assert session.editing_mode == EditingMode.VI

    def test_session_has_history(self):
        """Session is configured with FileHistory."""
        from prompt_toolkit.history import FileHistory
        from deephaven_cli.repl.prompt.session import create_prompt_session

        client = MagicMock()
        client.tables = []
        session = create_prompt_session(client, 10000)

        assert isinstance(session.history, FileHistory)

    def test_history_path_is_dh_history(self):
        """History file path is ~/.dh_history."""
        from prompt_toolkit.history import FileHistory
        from deephaven_cli.repl.prompt.session import create_prompt_session

        client = MagicMock()
        client.tables = []
        session = create_prompt_session(client, 10000)

        expected_path = str(Path.home() / ".dh_history")
        assert session.history.filename == expected_path

    def test_session_has_auto_suggest(self):
        """Session is configured with AutoSuggestFromHistory."""
        from prompt_toolkit.auto_suggest import AutoSuggestFromHistory
        from deephaven_cli.repl.prompt.session import create_prompt_session

        client = MagicMock()
        client.tables = []
        session = create_prompt_session(client, 10000)

        assert isinstance(session.auto_suggest, AutoSuggestFromHistory)

    def test_session_has_history_search_enabled(self):
        """Session has history search enabled."""
        from deephaven_cli.repl.prompt.session import create_prompt_session

        client = MagicMock()
        client.tables = []
        session = create_prompt_session(client, 10000)

        assert session.enable_history_search is True

    def test_session_has_mouse_support(self):
        """Session has mouse support enabled."""
        from deephaven_cli.repl.prompt.session import create_prompt_session

        client = MagicMock()
        client.tables = []
        session = create_prompt_session(client, 10000)

        assert session.mouse_support is True

    def test_session_is_multiline(self):
        """Session has multiline enabled."""
        from deephaven_cli.repl.prompt.session import create_prompt_session

        client = MagicMock()
        client.tables = []
        session = create_prompt_session(client, 10000)

        assert session.multiline is True
