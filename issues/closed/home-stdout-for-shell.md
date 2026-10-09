# matrix home must publish its path on stdout

Status: closed — discovered during the v0.1.54 run-submit qualification.

The v0.1.53 command used Cobra `Println`, whose default destination here was
stderr. Displaying the path worked, but `MATRIX_HOME="$(matrix home)"` captured
an empty string and broke the Wiki's local notification-socket example.

The command now writes to `cmd.OutOrStdout()`. The native compiled CLI smoke
checks the captured path before submitting its async task; this runs on the
Linux/macOS/Windows PAL workflow. The application home resolution is unchanged.
