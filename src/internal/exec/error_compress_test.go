package exec

import (
	"strings"
	"testing"
)

func TestCompressError_DHEngineError(t *testing.T) {
	// Real error output from dh exec --vm with a DH engine error
	input := `Traceback (most recent call last):
  File "<string>", line 23, in <module>
  File "<string>", line 1
    from deephaven import empty_table
    ^^^^
SyntaxError: invalid syntax

During handling of the above exception, another exception occurred:

Traceback (most recent call last):
  File "/usr/local/lib/python3.10/dist-packages/deephaven/table.py", line 1317, in update
    return Table(j_table=self.j_table.update(*formulas))
RuntimeError: io.deephaven.engine.table.impl.NoSuchColumnException: Unknown column names [X], available column names are []
	at io.deephaven.engine.table.impl.select.SourceColumn.initDef(SourceColumn.java:64)
	at io.deephaven.engine.table.impl.select.SelectColumn.initDef(SelectColumn.java:148)
	at io.deephaven.engine.table.impl.select.analyzers.SelectAndViewAnalyzer.createContext(SelectAndViewAnalyzer.java:128)
	at io.deephaven.engine.table.impl.QueryTable.lambda$selectOrUpdate$37(QueryTable.java:1661)

The above exception was the direct cause of the following exception:

Traceback (most recent call last):
  File "<string>", line 25, in <module>
  File "<string>", line 3, in <module>
  File "/usr/local/lib/python3.10/dist-packages/deephaven/table.py", line 1319, in update
    raise DHError(e, "table update operation failed.") from e
deephaven.dherror.DHError: table update operation failed. : RuntimeError: io.deephaven.engine.table.impl.NoSuchColumnException: Unknown column names [X], available column names are []`

	result := compressError(input, "basic_error.py", false)
	t.Logf("Result:\n%s", result)

	// Should contain the user's file reference with the filename
	if !strings.Contains(result, `"basic_error.py"`) {
		t.Errorf("expected filename replacement")
	}
	// Should contain the DHError
	if !strings.Contains(result, "DHError") {
		t.Errorf("expected DHError in output")
	}
	// Should contain root cause
	if !strings.Contains(result, "Caused by: NoSuchColumnException") {
		t.Errorf("expected Caused by: NoSuchColumnException")
	}
	// Should NOT contain Java stack traces
	if strings.Contains(result, "\tat io.deephaven") {
		t.Errorf("expected Java traces stripped")
	}
	// Should NOT contain SyntaxError
	if strings.Contains(result, "SyntaxError") {
		t.Errorf("expected eval SyntaxError stripped")
	}
	// Should NOT contain wrapper frames
	if strings.Contains(result, "line 23") || strings.Contains(result, "line 25") {
		t.Errorf("expected wrapper frames stripped")
	}
	// Should NOT contain DH library frames
	if strings.Contains(result, "table.py") {
		t.Errorf("expected DH library frames stripped")
	}
	// Should NOT contain Traceback header
	if strings.Contains(result, "Traceback") {
		t.Errorf("expected Traceback header stripped")
	}
}

func TestCompressError_RealOutput(t *testing.T) {
	// Actual error output observed from a live dh exec --vm run.
	// This has multiple independent Traceback blocks without chain headers.
	input := `Traceback (most recent call last):
  File "<string>", line 23, in <module>
  File "<string>", line 1
    from deephaven import empty_table
    ^^^^
SyntaxError: invalid syntax

During handling of the above exception, another exception occurred:

Traceback (most recent call last):
  File "/usr/local/lib/python3.10/dist-packages/deephaven/table.py", line 1317, in update
    return Table(j_table=self.j_table.update(*formulas))
RuntimeError: io.deephaven.engine.table.impl.NoSuchColumnException: Unknown column names [X], available column names are []
	at io.deephaven.engine.table.impl.select.SourceColumn.initDef(SourceColumn.java:64)
	at io.deephaven.engine.table.impl.select.SelectColumn.initDef(SelectColumn.java:148)
	at io.deephaven.engine.table.impl.select.analyzers.SelectAndViewAnalyzer.createContext(SelectAndViewAnalyzer.java:128)
	at io.deephaven.engine.table.impl.QueryTable.lambda$selectOrUpdate$37(QueryTable.java:1661)
	at io.deephaven.engine.table.impl.perf.QueryPerformanceRecorder.withNugget(QueryPerformanceRecorder.java:390)
	at io.deephaven.engine.table.impl.QueryTable.lambda$selectOrUpdate$38(QueryTable.java:1643)
	at io.deephaven.engine.table.impl.QueryTable.memoizeResult(QueryTable.java:3156)
	at io.deephaven.engine.table.impl.QueryTable.selectOrUpdate(QueryTable.java:1642)
	at io.deephaven.engine.table.impl.QueryTable.update(QueryTable.java:1621)
	at io.deephaven.engine.table.impl.QueryTable.update(QueryTable.java:101)
	at io.deephaven.api.TableOperationsDefaults.update(TableOperationsDefaults.java:94)
	at org.jpy.PyLib.executeCode(Native Method)
	at org.jpy.PyObject.executeCode(PyObject.java:133)
	at io.deephaven.engine.util.PythonEvaluatorJpy.evalScript(PythonEvaluatorJpy.java:73)
	at io.deephaven.integrations.python.PythonDeephavenSession.lambda$evaluate$1(PythonDeephavenSession.java:229)
	at io.deephaven.util.locks.FunctionalLock.doLockedInterruptibly(FunctionalLock.java:51)
	at io.deephaven.integrations.python.PythonDeephavenSession.evaluate(PythonDeephavenSession.java:229)
	at io.deephaven.engine.util.AbstractScriptSession.lambda$evaluateScript$0(AbstractScriptSession.java:168)
	at io.deephaven.engine.context.ExecutionContext.lambda$apply$0(ExecutionContext.java:196)
	at io.deephaven.engine.context.ExecutionContext.apply(ExecutionContext.java:207)
	at io.deephaven.engine.context.ExecutionContext.apply(ExecutionContext.java:195)
	at io.deephaven.engine.util.AbstractScriptSession.evaluateScript(AbstractScriptSession.java:168)
	at io.deephaven.engine.util.DelegatingScriptSession.evaluateScript(DelegatingScriptSession.java:77)
	at io.deephaven.engine.util.ScriptSession.evaluateScript(ScriptSession.java:123)
	at io.deephaven.server.console.ConsoleServiceGrpcImpl.lambda$executeCommand$7(ConsoleServiceGrpcImpl.java:204)
	at io.deephaven.server.session.SessionState$ExportObject.doExport(SessionState.java:1000)
	at java.base/java.util.concurrent.Executors$RunnableAdapter.call(Executors.java:539)
	at java.base/java.util.concurrent.FutureTask.run(FutureTask.java:264)
	at java.base/java.util.concurrent.ThreadPoolExecutor.runWorker(ThreadPoolExecutor.java:1136)
	at java.base/java.util.concurrent.ThreadPoolExecutor$Worker.run(ThreadPoolExecutor.java:635)
	at io.deephaven.server.runner.scheduler.SchedulerModule$ThreadFactory.lambda$newThread$0(SchedulerModule.java:100)
	at org.jpy.PyLib.callAndReturnObject(Native Method)
	at org.jpy.PyObject.call(PyObject.java:444)
	at io.deephaven.server.console.python.DebuggingInitializer.lambda$createInitializer$0(DebuggingInitializer.java:46)
	at java.base/java.lang.Thread.run(Thread.java:840)


The above exception was the direct cause of the following exception:

Traceback (most recent call last):
  File "<string>", line 25, in <module>
  File "<string>", line 3, in <module>
  File "/usr/local/lib/python3.10/dist-packages/deephaven/table.py", line 1319, in update
    raise DHError(e, "table update operation failed.") from e
deephaven.dherror.DHError: table update operation failed. : RuntimeError: io.deephaven.engine.table.impl.NoSuchColumnException: Unknown column names [X], available column names are []
Traceback (most recent call last):
  File "<string>", line 23, in <module>
  File "<string>", line 1
    from deephaven import empty_table
    ^^^^
SyntaxError: invalid syntax

During handling of the above exception, another exception occurred:

Traceback (most recent call last):
  File "/usr/local/lib/python3.10/dist-packages/deephaven/table.py", line 1317, in update
    return Table(j_table=self.j_table.update(*formulas))
RuntimeError: io.deephaven.engine.table.impl.NoSuchColumnException: Unknown column names [X], available column names are []
	at io.deephaven.engine.table.impl.select.SourceColumn.initDef(SourceColumn.java:64)
	at io.deephaven.engine.table.impl.select.SelectColumn.initDef(SelectColumn.java:148)
	at io.deephaven.engine.table.impl.select.analyzers.SelectAndViewAnalyzer.createContext(SelectAndViewAnalyzer.java:128)
	at io.deephaven.engine.table.impl.QueryTable.lambda$selectOrUpdate$37(QueryTable.java:1661)
	at io.deephaven.engine.table.impl.perf.QueryPerformanceRecorder.withNugget(QueryPerformanceRecorder.java:390)
	at io.deephaven.engine.table.impl.QueryTable.lambda$selectOrUpdate$38(QueryTable.java:1643)
	at io.deephaven.engine.table.impl.QueryTable.memoizeResult(QueryTable.java:3156)
	at io.deephaven.engine.table.impl.QueryTable.selectOrUpdate(QueryTable.java:1642)
	at io.deephaven.engine.table.impl.QueryTable.update(QueryTable.java:1621)
	at io.deephaven.engine.table.impl.QueryTable.update(QueryTable.java:101)
	at io.deephaven.api.TableOperationsDefaults.update(TableOperationsDefaults.java:94)
	at org.jpy.PyLib.executeCode(Native Method)
	at org.jpy.PyObject.executeCode(PyObject.java:133)
	at io.deephaven.engine.util.PythonEvaluatorJpy.evalScript(PythonEvaluatorJpy.java:73)
	at io.deephaven.integrations.python.PythonDeephavenSession.lambda$evaluate$1(PythonDeephavenSession.java:229)
	at io.deephaven.util.locks.FunctionalLock.doLockedInterruptibly(FunctionalLock.java:51)
	at io.deephaven.integrations.python.PythonDeephavenSession.evaluate(PythonDeephavenSession.java:229)
	at io.deephaven.engine.util.AbstractScriptSession.lambda$evaluateScript$0(AbstractScriptSession.java:168)
	at io.deephaven.engine.context.ExecutionContext.lambda$apply$0(ExecutionContext.java:196)
	at io.deephaven.engine.context.ExecutionContext.apply(ExecutionContext.java:207)
	at io.deephaven.engine.context.ExecutionContext.apply(ExecutionContext.java:195)
	at io.deephaven.engine.util.AbstractScriptSession.evaluateScript(AbstractScriptSession.java:168)
	at io.deephaven.engine.util.DelegatingScriptSession.evaluateScript(DelegatingScriptSession.java:77)
	at io.deephaven.engine.util.ScriptSession.evaluateScript(ScriptSession.java:123)
	at io.deephaven.server.console.ConsoleServiceGrpcImpl.lambda$executeCommand$7(ConsoleServiceGrpcImpl.java:204)
	at io.deephaven.server.session.SessionState$ExportObject.doExport(SessionState.java:1000)
	at java.base/java.util.concurrent.Executors$RunnableAdapter.call(Executors.java:539)
	at java.base/java.util.concurrent.FutureTask.run(FutureTask.java:264)
	at java.base/java.util.concurrent.ThreadPoolExecutor.runWorker(ThreadPoolExecutor.java:1136)
	at java.base/java.util.concurrent.ThreadPoolExecutor$Worker.run(ThreadPoolExecutor.java:635)
	at io.deephaven.server.runner.scheduler.SchedulerModule$ThreadFactory.lambda$newThread$0(SchedulerModule.java:100)
	at org.jpy.PyLib.callAndReturnObject(Native Method)
	at org.jpy.PyObject.call(PyObject.java:444)
	at io.deephaven.server.console.python.DebuggingInitializer.lambda$createInitializer$0(DebuggingInitializer.java:46)
	at java.base/java.lang.Thread.run(Thread.java:840)


The above exception was the direct cause of the following exception:

Traceback (most recent call last):
  File "<string>", line 25, in <module>
  File "<string>", line 3, in <module>
  File "/usr/local/lib/python3.10/dist-packages/deephaven/table.py", line 1319, in update
    raise DHError(e, "table update operation failed.") from e
deephaven.dherror.DHError: table update operation failed. : RuntimeError: io.deephaven.engine.table.impl.NoSuchColumnException: Unknown column names [X], available column names are []`

	result := compressError(input, "basic_error.py", false)
	t.Logf("Compressed %d lines → %d lines:\n%s",
		len(strings.Split(input, "\n")),
		len(strings.Split(result, "\n")),
		result)

	// Target output is just 3 lines:
	//   File "basic_error.py", line 3
	// deephaven.dherror.DHError: table update operation failed.
	//   Caused by: NoSuchColumnException: Unknown column names [X], available column names are []
	resultLines := strings.Split(result, "\n")
	if len(resultLines) > 4 {
		t.Errorf("expected ≤4 lines, got %d", len(resultLines))
	}

	if !strings.Contains(result, `"basic_error.py", line 3`) {
		t.Errorf("expected user frame with filename and line 3")
	}
	if !strings.Contains(result, "DHError: table update operation failed.") {
		t.Errorf("expected DHError message")
	}
	if !strings.Contains(result, "Caused by: NoSuchColumnException") {
		t.Errorf("expected root cause")
	}
	if strings.Contains(result, "SyntaxError") {
		t.Errorf("should not contain eval SyntaxError")
	}
	if strings.Contains(result, "\tat ") {
		t.Errorf("should not contain Java traces")
	}
	if strings.Contains(result, "Traceback") {
		t.Errorf("should not contain Traceback header")
	}
	if strings.Contains(result, "table.py") {
		t.Errorf("should not contain DH library frame")
	}
}

func TestCompressError_VerbosePassthrough(t *testing.T) {
	input := "Traceback (most recent call last):\n  some error"
	result := compressError(input, "test.py", true)
	if result != input {
		t.Errorf("verbose mode should pass through unchanged")
	}
}

func TestCompressError_Empty(t *testing.T) {
	if compressError("", "test.py", false) != "" {
		t.Error("empty input should return empty")
	}
}

func TestCompressError_SimplePythonError(t *testing.T) {
	input := `Traceback (most recent call last):
  File "<string>", line 25, in <module>
  File "<string>", line 5, in <module>
NameError: name 'undefined_var' is not defined`

	result := compressError(input, "test.py", false)
	t.Logf("Result:\n%s", result)

	if !strings.Contains(result, `"test.py", line 5`) {
		t.Errorf("expected user frame preserved")
	}
	if strings.Contains(result, "line 25") {
		t.Errorf("expected wrapper frame stripped")
	}
	if !strings.Contains(result, "NameError") {
		t.Errorf("expected error preserved")
	}
}

func TestCompressError_PureSyntaxError(t *testing.T) {
	// When SyntaxError is the ONLY error (not from eval preamble), keep it
	input := `Traceback (most recent call last):
  File "<string>", line 23, in <module>
  File "<string>", line 2
    def foo(
           ^
SyntaxError: unexpected EOF while parsing`

	result := compressError(input, "test.py", false)
	t.Logf("Result:\n%s", result)

	if !strings.Contains(result, "SyntaxError") {
		t.Errorf("expected SyntaxError preserved when it's the real error")
	}
	if !strings.Contains(result, `"test.py", line 2`) {
		t.Errorf("expected user frame with line 2")
	}
}
