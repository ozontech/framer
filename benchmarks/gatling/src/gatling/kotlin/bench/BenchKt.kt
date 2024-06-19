package bench

import com.github.phisgr.gatling.kt.grpc.*
import bench.BenchmarkGrpc
import bench.BenchmarkOuterClass.BlankGetRequest
import io.gatling.javaapi.core.CoreDsl.*
import io.gatling.javaapi.core.*
import io.grpc.ManagedChannelBuilder

class BenchKt : Simulation() {

    private val grpcConf = grpc(ManagedChannelBuilder.forAddress("localhost", 9090).usePlaintext())

    private fun request(name: String) = grpc(name)
            .rpc(BenchmarkGrpc.getBlankGetMethod())
            .payload(BlankGetRequest::newBuilder) {
                build()
            }

    private val scn = scenario("Play Ping Pong").exec(
            request("Send message")
    )

    init {
        setUp(
                scn.injectOpen(
                        rampUsersPerSec(10.0).to(100000.0).during(40), //line
                        constantUsersPerSec(100000.0).during(20), //const
                ).protocols(grpcConf.shareChannel())
        ).maxDuration(62) //limit
    }
}

