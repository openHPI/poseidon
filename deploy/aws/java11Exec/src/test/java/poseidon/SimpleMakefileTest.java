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
    Map<String, String> files = new HashMap<>();
    files.put("Makefile", SuccessfulMakefile);
    files.put("org/example/RecursiveMath.java", RecursiveMathContent);

    try {
      String command = "make run";
      SimpleMakefile makefile = new SimpleMakefile(files);
      String cmd = makefile.parseCommand(command);

      assertEquals("javac org/example/RecursiveMath.java && java org/example/RecursiveMath", cmd);
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
  public void withNotSupportedMakefile() {
    Map<String, String> files = new HashMap<>();
    files.put("Makefile", NotSupportedMakefile);
    files.put("org/example/RecursiveMath.java", RecursiveMathContent);

    try {
      String command = "make run";
      SimpleMakefile makefile = new SimpleMakefile(files);
      makefile.parseCommand(command);
      fail();
    } catch (NoMakefileFoundException | NoMakeCommandException ignored) {
      fail();
    } catch (InvalidMakefileException ignored) {}
  }
}