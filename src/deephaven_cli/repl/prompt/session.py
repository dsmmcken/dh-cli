"""PromptSession configuration for Deephaven REPL."""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

from prompt_toolkit import PromptSession
from prompt_toolkit.auto_suggest import AutoSuggestFromHistory
from prompt_toolkit.enums import EditingMode
from prompt_toolkit.history import FileHistory
from prompt_toolkit.lexers import PygmentsLexer
from pygments.lexers.python import PythonLexer

from .keybindings import create_key_bindings
from .toolbar import create_toolbar

if TYPE_CHECKING:
    from deephaven_cli.client import DeephavenClient


def create_prompt_session(
    client: DeephavenClient, port: int, vi_mode: bool = False
) -> PromptSession:
    """Create configured PromptSession with all enhancements."""
    return PromptSession(
        history=FileHistory(str(Path.home() / ".dh_history")),
        lexer=PygmentsLexer(PythonLexer),
        auto_suggest=AutoSuggestFromHistory(),
        bottom_toolbar=create_toolbar(client, port),
        key_bindings=create_key_bindings(),
        enable_history_search=True,
        mouse_support=True,
        editing_mode=EditingMode.VI if vi_mode else EditingMode.EMACS,
        multiline=True,
    )
