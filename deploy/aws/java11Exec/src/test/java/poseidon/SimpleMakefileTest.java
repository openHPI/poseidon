package poseidon;

import org.junit.Test;

import java.nio.charset.StandardCharsets;
import java.util.Base64;
import java.util.HashMap;
import java.util.Map;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.fail;
import static poseidon.AppTest.RecursiveMathContent;


public class SimpleMakefileTest {
  static final String SuccessfulMakefile = Base64.getEncoder().encodeToString(
          ("run:\n" +
                  "\tjavac org/example/RecursiveMath.java\n" +
                  "\tjava org/example/RecursiveMath\n" +
                  "\n" +
          "test:\n" +
                  "\techo Hi\n"
          ).getBytes(StandardCharsets.UTF_8));

  static final String SuccessfulWindowsMakefile = Base64.getEncoder().encodeToString(
          ("run:\r\n" +
                  "\tjavac org/example/RecursiveMath.java\r\n" +
                  "\tjava org/example/RecursiveMath\r\n" +
                  "\r\n" +
                  "test:\r\n" +
                  "\techo Hi\r\n"
          ).getBytes(StandardCharsets.UTF_8));

  static final String SuccessfulMakefileWithAtSymbol = Base64.getEncoder().encodeToString(
          ("run:\r\n" +
                  "\t@javac org/example/RecursiveMath.java\r\n" +
                  "\t@java org/example/RecursiveMath\r\n"
          ).getBytes(StandardCharsets.UTF_8));

  static final String SuccessfulMakefileWithAssignments = Base64.getEncoder().encodeToString(
          ("test:\n" +
                  "\tjavac -encoding utf8 ${FILENAME}\n" +
                  "\tjava -Dfile.encoding=UTF8 ${CLASS_NAME}\n"
          ).getBytes(StandardCharsets.UTF_8));

  static final String SuccessfulMakefileWithComment = Base64.getEncoder().encodeToString(
          ("run:\r\n" +
                  "\t@javac org/example/RecursiveMath.java\r\n" +
                  "\t@java org/example/RecursiveMath\r\n" +
                  "\t#exit\r\n"
          ).getBytes(StandardCharsets.UTF_8));

  static final String NotSupportedMakefile = Base64.getEncoder().encodeToString(
          ("run: test\n" +
                  "\tjavac org/example/RecursiveMath.java\n" +
                  "\tjava org/example/RecursiveMath\n" +
                  "\n" +
          "test:\n" +
                  "\techo Hi\n"
          ).getBytes(StandardCharsets.UTF_8));

  @Test
  public void sucessfullMake() {
    parseRunCommandOfMakefile(SuccessfulMakefile);
  }

  @Test
  public void sucessfullMakeWithCR() {
    parseRunCommandOfMakefile(SuccessfulWindowsMakefile);
  }

  // We remove [the @ Symbol](https://www.gnu.org/software/make/manual/make.html#Echoing)
  // as the command itself is never written to stdout with this implementation.
  @Test
  public void sucessfullMakeWithAtSymbol() {
    parseRunCommandOfMakefile(SuccessfulMakefileWithAtSymbol);
  }

  // We remove [any comments with #](https://www.gnu.org/software/make/manual/make.html#Recipe-Syntax)
  // as they are normally ignored / echoed with most shells.
  @Test
  public void sucessfullMakeWithComment() {
    parseRunCommandOfMakefile(SuccessfulMakefileWithComment);
  }

  private String wrapFullCommand(String command) {
    return "env CODEOCEAN=true /bin/bash -c \"" + command + "\"";
  }

  private void parseRunCommandOfMakefile(String makefileB64) {
    Map<String, String> files = new HashMap<>();
    files.put("Makefile", makefileB64);
    files.put("org/example/RecursiveMath.java", RecursiveMathContent);

    try {
      String command = wrapFullCommand("make run");
      SimpleMakefile makefile = new SimpleMakefile(files);
      String cmd = makefile.parseCommand(command);

      assertEquals(wrapFullCommand("javac org/example/RecursiveMath.java && java org/example/RecursiveMath"), cmd);
    } catch (NoMakefileFoundException | InvalidMakefileException | NoMakeCommandException ignored) {
      fail();
    }
  }

  @Test
  public void withoutMake() {
    Map<String, String> files = new HashMap<>();
    files.put("Makefile", SuccessfulMakefile);
    files.put("org/example/RecursiveMath.java", RecursiveMathContent);

    try {
      String command = "javac org/example/RecursiveMath.java";
      SimpleMakefile make = new SimpleMakefile(files);
      make.parseCommand(command);
      fail();
    } catch (NoMakefileFoundException | InvalidMakefileException ignored) {
      fail();
    } catch (NoMakeCommandException ignored) {}
  }

  @Test
  public void sucessfullMakeWithAssignments() {
    Map<String, String> files = new HashMap<>();
    files.put("Makefile", SuccessfulMakefileWithAssignments);
    files.put("org/example/RecursiveMath.java", RecursiveMathContent);

    try {
      String command = wrapFullCommand("make test CLASS_NAME=\"RecursiveMath\" FILENAME=\"RecursiveMath-Test.java\"");
      SimpleMakefile make = new SimpleMakefile(files);
      String cmd = make.parseCommand(command);

      assertEquals(wrapFullCommand("javac -encoding utf8 RecursiveMath-Test.java && " +
              "java -Dfile.encoding=UTF8 RecursiveMath"), cmd);
    } catch (NoMakefileFoundException | InvalidMakefileException | NoMakeCommandException ignored) {
      fail();
    }
  }

  @Test
  public void withNotSupportedMakefile() {
    Map<String, String> files = new HashMap<>();
    files.put("Makefile", NotSupportedMakefile);
    files.put("org/example/RecursiveMath.java", RecursiveMathContent);

    try {
      String command = wrapFullCommand("make run");
      SimpleMakefile makefile = new SimpleMakefile(files);
      makefile.parseCommand(command);
      fail();
    } catch (NoMakefileFoundException | NoMakeCommandException ignored) {
      fail();
    } catch (InvalidMakefileException ignored) {}
  }

  @Test
  public void withBeforeAndAfterStatements() {
    Map<String, String> files = new HashMap<>();
    files.put("Makefile", Base64.getEncoder().encodeToString(("run:\n\t@echo TRAAAIIN\n").getBytes(StandardCharsets.UTF_8)));

    try {
      String command = wrapFullCommand("echo \"Look it's a\" && sl && make run && echo WOW");
      SimpleMakefile makefile = new SimpleMakefile(files);
      String cmd = makefile.parseCommand(command);

      assertEquals(wrapFullCommand("echo \"Look it's a\" && sl && echo TRAAAIIN && echo WOW"), cmd);
    } catch (NoMakefileFoundException | InvalidMakefileException | NoMakeCommandException ignored) {
      fail();
    }
  }
}
