import static us.abstracta.jmeter.javadsl.JmeterDsl.*;
import static us.abstracta.jmeter.javadsl.dashboard.DashboardVisualizer.dashboardVisualizer;

import java.io.IOException;
import java.time.Duration;
import java.util.*;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;

//import benchmark.BenchmarkGrpc;
//import benchmark.BenchmarkOuterClass;
import io.grpc.ManagedChannel;
import io.grpc.ManagedChannelBuilder;
import io.netty.channel.nio.NioEventLoopGroup;
import org.junit.jupiter.api.Test;
import test.api.Api;
import test.api.TestApiGrpc;
import us.abstracta.jmeter.javadsl.core.TestPlanStats;
import us.abstracta.jmeter.javadsl.core.threadgroups.RpsThreadGroup;

public class PerformanceIT {
//    BenchmarkGrpc.TestApiBlockingStub.

    @Test
    public void testPerformance() throws IOException {
        String host = System.getProperty("target.host");
        System.out.println(host);
        String getenv = System.getProperty("target.port");
        System.out.println(getenv);
        int port = Integer.parseInt(getenv);
        int maxThreads = Integer.parseInt(System.getProperty("max.threads"));
        int targetRPS = Integer.parseInt(System.getProperty("target.rps"));
        int numClients = Integer.parseInt(System.getProperty("grpc.numclients"));

        Map<String, TestApiGrpc.TestApiBlockingStub> stubs = new HashMap<>();
        for (int i = 0; i < numClients; i++) {
            ManagedChannel client = ManagedChannelBuilder
                    .forAddress(host, port)
                    .keepAliveTime(5, TimeUnit.SECONDS)
                    .usePlaintext().build();
            TestApiGrpc.TestApiBlockingStub stub = TestApiGrpc.newBlockingStub(client);
            stub.withCompression(System.getProperty("grpc.compression"));
            stub.withExecutor(Executors.newCachedThreadPool());
            stubs.put(String.valueOf(i), stub);
        }
        Random generator = new Random();
        Object[] clients = stubs.values().toArray();

        Api.TestRequest trueRequest = Api.TestRequest.newBuilder().setField("true").build();

        RpsThreadGroup children = rpsThreadGroup()
                .maxThreads(maxThreads)
                .rampTo(targetRPS, Duration.ofSeconds(120))
                .rampToAndHold(targetRPS, Duration.ofSeconds(120), Duration.ofSeconds(120))
                .children(
                        jsr223Sampler(vars -> {
                            try {
                                TestApiGrpc.TestApiBlockingStub value =
                                        (TestApiGrpc.TestApiBlockingStub) clients[generator.nextInt(clients.length)];
                                Api.EmptyResponse resp = value.test(trueRequest);
                                vars.sampleResult.setSentBytes(trueRequest.getSerializedSize());
                                vars.sampleResult.setBytes(resp.getSerializedSize());
                            } catch (Exception e) {
                                vars.sampleResult.setResponseCode("500");
                            }
                        })
                );
        TestPlanStats stats = testPlan(
                children,
//                dashboardVisualizer(),

                htmlReporter("./")).run();


    }

}